package execution

import (
	"pfi/sensorbee/sensorbee/bql/udf"
	"pfi/sensorbee/sensorbee/core"
	"pfi/sensorbee/sensorbee/data"
)

type groupbyExecutionPlan struct {
	streamRelationStreamExecutionPlan
}

// tmpGroupData is an intermediate data structure to represent
// a set of rows that have the same values for GROUP BY columns.
type tmpGroupData struct {
	// this is the group (e.g. [1, "toy"]), where the values are
	// in order of the items in the GROUP BY clause
	group data.Array
	// for each aggregate function, we hold an array with the
	// input values.
	aggData map[string][]data.Value
	// as per our assumptions about grouping, the non-aggregation
	// data should be identical within every group
	nonAggData data.Map
}

// CanBuildGroupbyExecutionPlan checks whether the given statement
// allows to use an groupbyExecutionPlan.
func CanBuildGroupbyExecutionPlan(lp *LogicalPlan, reg udf.FunctionRegistry) bool {
	return lp.GroupingStmt && lp.Having == nil
}

// groupbyExecutionPlan is a simple plan that follows the
// theoretical processing model. It supports only statements
// that use aggregation.
//
// After each tuple arrives,
// - compute the contents of the current window using the
//   specified window size/type,
// - perform a SELECT query on that data,
// - compute the data that need to be emitted by comparison with
//   the previous run's results.
func NewGroupbyExecutionPlan(lp *LogicalPlan, reg udf.FunctionRegistry) (ExecutionPlan, error) {
	underlying, err := newStreamRelationStreamExecutionPlan(lp, reg)
	if err != nil {
		return nil, err
	}
	return &groupbyExecutionPlan{
		*underlying,
	}, nil
}

// Process takes an input tuple and returns a slice of Map values that
// correspond to the results of the query represented by this execution
// plan. Note that the order of items in the returned slice is undefined
// and cannot be relied on.
func (ep *groupbyExecutionPlan) Process(input *core.Tuple) ([]data.Map, error) {
	return ep.process(input, ep.performQueryOnBuffer)
}

// performQueryOnBuffer executes a SELECT query on the data of the tuples
// currently stored in the buffer. The query results (which is a set of
// data.Value, not core.Tuple) is stored in ep.curResults. The data
// that was stored in ep.curResults before this method was called is
// moved to ep.prevResults. Note that the order of values in ep.curResults
// is undefined.
//
// In case of an error the contents of ep.curResults will still be
// the same as before the call (so that the next run performs as
// if no error had happened), but the contents of ep.curResults are
// undefined.
//
// Currently performQueryOnBuffer can only perform SELECT ... WHERE ...
// queries without aggregate functions, GROUP BY, or HAVING clauses.
func (ep *groupbyExecutionPlan) performQueryOnBuffer() error {
	// reuse the allocated memory
	output := ep.prevResults[0:0]
	// remember the previous results
	ep.prevResults = ep.curResults

	rollback := func() {
		// NB. ep.prevResults currently points to an slice with
		//     results from the previous run. ep.curResults points
		//     to the same slice. output points to a different slice
		//     with a different underlying array.
		//     in the next run, output will be reusing the underlying
		//     storage of the current ep.prevResults to hold results.
		//     therefore when we leave this function we must make
		//     sure that ep.prevResults and ep.curResults have
		//     different underlying arrays or ISTREAM/DSTREAM will
		//     return wrong results.
		ep.prevResults = output
	}

	// groups holds one item for every combination of values that
	// appear in the GROUP BY clause
	groups := []tmpGroupData{}

	// findOrCreateGroup looks up the group that has the given
	// groupValues in the `groups` list. if there is no such
	// group, a new one is created and a copy of the given map
	// is used as a representative of this group's values.
	findOrCreateGroup := func(groupValues []data.Value, d data.Map) (*tmpGroupData, error) {
		eq := Equal(binOp{}).(*compBinOp).cmpOp
		groupValuesArr := data.Array(groupValues)
		// find the correct group
		groupIdx := -1
		for i, groupData := range groups {
			equals, err := eq(groupData.group, groupValuesArr)
			if err != nil {
				return nil, err
			}
			if equals {
				groupIdx = i
				break
			}
		}
		// if there is no such group, create one
		if groupIdx < 0 {
			newGroup := tmpGroupData{
				// the values that make up this group
				groupValues,
				// the input values of the aggregate functions
				map[string][]data.Value{},
				// a representative set of values for this group for later evaluation
				// TODO actually we don't need the whole map,
				//      just the parts common to the whole group
				d.Copy(),
			}
			// initialize the map with the aggregate function inputs
			for _, proj := range ep.projections {
				for key := range proj.aggrEvals {
					newGroup.aggData[key] = make([]data.Value, 0, 1)
				}
			}
			groups = append(groups, newGroup)
			groupIdx = len(groups) - 1
		}

		// return a pointer to the (found or created) group
		return &groups[groupIdx], nil
	}

	// we need to make a cross product of the data in all buffers,
	// combine it to get an input like
	//  {"streamA": {data}, "streamB": {data}, "streamC": {data}}
	// and then run filter/projections on each of this items

	dataHolder := data.Map{}

	// function to evaluate filter on the input data and do the computations
	// that are required on each input tuple. (those computations differ
	// depending on whether we are in grouping mode or not.)
	evalItem := func(d data.Map) error {
		// evaluate filter condition and convert to bool
		if ep.filter != nil {
			filterResult, err := ep.filter.Eval(d)
			if err != nil {
				return err
			}
			filterResultBool, err := data.ToBool(filterResult)
			if err != nil {
				return err
			}
			// if it evaluated to false, do not further process this tuple
			// (ToBool also evalutes the NULL value to false, so we don't
			// need to treat this specially)
			if !filterResultBool {
				return nil
			}
		}

		// now compute the expressions in the GROUP BY to find the correct
		// group to append to
		itemGroupValues := make([]data.Value, len(ep.groupList))
		for i, eval := range ep.groupList {
			// ordinary "flat" expression
			value, err := eval.Eval(d)
			if err != nil {
				return err
			}
			itemGroupValues[i] = value
		}

		itemGroup, err := findOrCreateGroup(itemGroupValues, d)
		if err != nil {
			return err
		}

		// now compute all the input data for the aggregate functions,
		// e.g. for `SELECT count(a) + max(b/2)`, compute `a` and `b/2`
		for _, proj := range ep.projections {
			if proj.hasAggregate {
				// this column involves an aggregate function, but there
				// may be multiple ones
				for key, agg := range proj.aggrEvals {
					value, err := agg.aggrEval.Eval(d)
					if err != nil {
						return err
					}
					// now we need to store this value in the output map
					itemGroup.aggData[key] = append(itemGroup.aggData[key], value)
				}
			}
		}
		return nil
	}

	evalGroup := func(group *tmpGroupData) error {
		result := data.Map(make(map[string]data.Value, len(ep.projections)))
		for _, proj := range ep.projections {
			// compute aggregate values
			if proj.hasAggregate {
				// this column involves an aggregate function, but there
				// may be multiple ones
				for key, agg := range proj.aggrEvals {
					aggregateInputs := group.aggData[key]
					_ = agg.aggrFun
					// TODO use the real function, not poor man's "count",
					//      and also return an error on failure
					result := data.Int(len(aggregateInputs))
					group.nonAggData[key] = result
					delete(group.aggData, key)
				}
			}
			// now evaluate this projection on  the flattened data
			value, err := proj.evaluator.Eval(group.nonAggData)
			if err != nil {
				return err
			}
			if err := assignOutputValue(result, proj.alias, value); err != nil {
				return err
			}
		}
		output = append(output, result)
		return nil
	}

	// Note: `ep.buffers` is a map, so iterating over its keys may yield
	// different results in every run of the program. We cannot expect
	// a consistent order in which evalItem is run on the items of the
	// cartesian product.
	allStreams := make([]string, 0, len(ep.buffers))
	for key := range ep.buffers {
		allStreams = append(allStreams, key)
	}
	if err := ep.processCartesianProduct(dataHolder, allStreams, evalItem); err != nil {
		rollback()
		return err
	}

	// if we arrive here, then the input for the aggregation functions
	// is in the `group` list and we need to compute aggregation and output.
	// TODO deal with the case of an empty list
	for _, group := range groups {
		if err := evalGroup(&group); err != nil {
			rollback()
			return err
		}
	}

	ep.curResults = output
	return nil
}
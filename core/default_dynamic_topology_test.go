package core

import (
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestDefaultDynamicTopologySetup(t *testing.T) {
	config := Configuration{TupleTraceEnabled: 1}
	ctx := newTestContext(config)

	Convey("Given a default dynamic topology", t, func() {
		dt := NewDefaultDynamicTopology(ctx, "dt1")
		t := dt.(*defaultDynamicTopology)
		Reset(func() {
			t.Stop()
		})

		dupNameTests := func(name string) {
			Convey("Then adding a source having the same name should fail", func() {
				_, err := t.AddSource(name, &DummySource{}, nil)
				So(err, ShouldNotBeNil)
			})

			Convey("Then adding a box having the same name should fail", func() {
				_, err := t.AddBox(name, &DummyBox{}, nil)
				So(err, ShouldNotBeNil)
			})

			Convey("Then adding a sink having the same name should fail", func() {
				_, err := t.AddSink(name, &DummySink{}, nil)
				So(err, ShouldNotBeNil)
			})
		}

		Convey("When stopping it without adding anything", func() {
			t.Stop()

			Convey("Then it should stop", func() {
				So(t.state.state, ShouldEqual, TSStopped)
			})

			Convey("Then adding a source to the stopped topology should fail", func() {
				_, err := t.AddSource("test_source", &DummySource{}, nil)
				So(err, ShouldNotBeNil)
			})

			Convey("Then adding a box to the stopped topology should fail", func() {
				_, err := t.AddBox("test_box", &DummyBox{}, nil)
				So(err, ShouldNotBeNil)
			})

			Convey("Then adding a sink to the stopped topology should fail", func() {
				_, err := t.AddSink("test_sink", &DummySink{}, nil)
				So(err, ShouldNotBeNil)
			})
		})

		Convey("When adding a source", func() {
			s := NewTupleIncrementalEmitterSource(freshTuples())
			sn, err := t.AddSource("source1", s, nil)
			So(err, ShouldBeNil)

			Convey("Then it should automatically run", func() {
				So(sn.State().Get(), ShouldEqual, TSRunning)
			})

			Convey("Then it should be able to stop", func() {
				So(sn.Stop(), ShouldBeNil)
				So(sn.State().Get(), ShouldEqual, TSStopped)
			})

			Convey("Then the topology should have it", func() {
				n, err := t.Source("source1")
				So(err, ShouldBeNil)
				So(n, ShouldPointTo, sn)
			})

			Convey("Then it can be obtained as a node", func() {
				n, err := t.Node("source1")
				So(err, ShouldBeNil)
				So(n, ShouldPointTo, sn)
			})

			dupNameTests("source1")
		})

		Convey("When adding a box", func() {
			b := newTerminateChecker(&DummyBox{})
			bn, err := t.AddBox("box1", b, nil)
			So(err, ShouldBeNil)

			Convey("Then it should automatically run", func() {
				So(bn.State().Get(), ShouldEqual, TSRunning)
			})

			Convey("Then it should be able to stop", func() {
				So(bn.Stop(), ShouldBeNil)
				So(bn.State().Get(), ShouldEqual, TSStopped)

				Convey("And it should be terminated", func() {
					So(b.terminateCnt, ShouldEqual, 1)
				})
			})

			Convey("Then Terminate should be called after stopping the topology", func() {
				So(t.Stop(), ShouldBeNil)
				So(b.terminateCnt, ShouldEqual, 1)
			})

			Convey("Then the topology should have it", func() {
				n, err := t.Box("box1")
				So(err, ShouldBeNil)
				So(n, ShouldPointTo, bn)
			})

			Convey("Then it can be obtained as a node", func() {
				n, err := t.Node("box1")
				So(err, ShouldBeNil)
				So(n, ShouldPointTo, bn)
			})

			dupNameTests("box1")
		})

		Convey("When adding a sink", func() {
			s := &DummySink{}
			sn, err := t.AddSink("sink1", s, nil)
			So(err, ShouldBeNil)

			Convey("Then it should automatically run", func() {
				So(sn.State().Get(), ShouldEqual, TSRunning)
			})

			Convey("Then the topology should have it", func() {
				n, err := t.Sink("sink1")
				So(err, ShouldBeNil)
				So(n, ShouldPointTo, sn)
			})

			Convey("Then it can be obtained as a node", func() {
				n, err := t.Node("sink1")
				So(err, ShouldBeNil)
				So(n, ShouldPointTo, sn)
			})

			Convey("Then it should be able to stop", func() {
				So(sn.Stop(), ShouldBeNil)
				So(sn.State().Get(), ShouldEqual, TSStopped)
			})

			dupNameTests("sink1")
		})

		Convey("When getting nonexistent node", func() {
			_, err := t.Node("source1")

			Convey("Then it shouldn't be found", func() {
				So(err, ShouldNotBeNil)
			})
		})
	})
}

func TestLinearDefaultDynamicTopology(t *testing.T) {
	// This test is written based on TestShutdownLinearDefaultStaticTopology
	config := Configuration{TupleTraceEnabled: 1}
	ctx := newTestContext(config)

	Convey("Given a simple linear topology", t, func() {
		/*
		 *   so -*--> b1 -*--> b2 -*--> si
		 */
		dt := NewDefaultDynamicTopology(ctx, "dt1")
		t := dt.(*defaultDynamicTopology)

		so := NewTupleIncrementalEmitterSource(freshTuples())
		son, err := t.AddSource("source", so, nil)
		So(err, ShouldBeNil)

		b1 := &BlockingForwardBox{cnt: 8}
		tc1 := newTerminateChecker(b1)
		bn1, err := t.AddBox("box1", tc1, nil)
		So(err, ShouldBeNil)
		So(bn1.Input("source", nil), ShouldBeNil)

		b2 := BoxFunc(forwardBox)
		tc2 := newTerminateChecker(b2)
		bn2, err := t.AddBox("box2", tc2, nil)
		So(err, ShouldBeNil)
		So(bn2.Input("box1", nil), ShouldBeNil)

		si := NewTupleCollectorSink()
		sic := &sinkCloseChecker{s: si}
		sin, err := t.AddSink("sink", sic, nil)
		So(err, ShouldBeNil)
		So(sin.Input("box2", nil), ShouldBeNil)

		checkPostCond := func() {
			Convey("Then the topology should be stopped", func() {
				So(t.state.Get(), ShouldEqual, TSStopped)
			})

			Convey("Then Box.Terminate should be called exactly once", func() {
				So(tc1.terminateCnt, ShouldEqual, 1)
				So(tc2.terminateCnt, ShouldEqual, 1)
			})

			Convey("Then Sink.Close should be called exactly once", func() {
				So(sic.closeCnt, ShouldEqual, 1)
			})
		}

		Convey("When getting registered nodes", func() {
			Reset(func() {
				t.Stop()
			})

			Convey("Then the topology should return all nodes", func() {
				ns := t.Nodes()
				So(len(ns), ShouldEqual, 4)
				So(ns["source"], ShouldPointTo, son)
				So(ns["box1"], ShouldPointTo, bn1)
				So(ns["box2"], ShouldPointTo, bn2)
				So(ns["sink"], ShouldPointTo, sin)
			})

			Convey("Then source should be able to be obtained", func() {
				s, err := t.Source("source")
				So(err, ShouldBeNil)
				So(s, ShouldPointTo, son)
			})

			Convey("Then source should be able to be obtained through Sources", func() {
				ss := t.Sources()
				So(len(ss), ShouldEqual, 1)
				So(ss["source"], ShouldPointTo, son)
			})

			Convey("Then box1 should be able to be obtained", func() {
				b, err := t.Box("box1")
				So(err, ShouldBeNil)
				So(b, ShouldPointTo, bn1)
			})

			Convey("Then box2 should be able to be obtained", func() {
				b, err := t.Box("box2")
				So(err, ShouldBeNil)
				So(b, ShouldPointTo, bn2)
			})

			Convey("Then all boxes should be able to be obtained at once", func() {
				bs := t.Boxes()
				So(len(bs), ShouldEqual, 2)
				So(bs["box1"], ShouldPointTo, bn1)
				So(bs["box2"], ShouldPointTo, bn2)
			})

			Convey("Then sink should be able to be obtained", func() {
				s, err := t.Sink("sink")
				So(err, ShouldBeNil)
				So(s, ShouldPointTo, sin)
			})

			Convey("Then sink should be able to be obtained through Sinks", func() {
				ss := t.Sinks()
				So(len(ss), ShouldEqual, 1)
				So(ss["sink"], ShouldPointTo, sin)
			})

			Convey("Then source cannot be obtained via wronge methods", func() {
				_, err := t.Box("source")
				So(err, ShouldNotBeNil)
				_, err = t.Sink("source")
				So(err, ShouldNotBeNil)
			})

			Convey("Then box1 cannot be obtained via wronge methods", func() {
				_, err := t.Source("box1")
				So(err, ShouldNotBeNil)
				_, err = t.Sink("box1")
				So(err, ShouldNotBeNil)
			})

			Convey("Then sink cannot be obtained via wronge methods", func() {
				_, err := t.Source("sink")
				So(err, ShouldNotBeNil)
				_, err = t.Box("sink")
				So(err, ShouldNotBeNil)
			})
		})

		Convey("When generating no tuples and call stop", func() {
			So(t.Stop(), ShouldBeNil)
			checkPostCond()
		})

		Convey("When generating some tuples and call stop before the sink receives a tuple", func() { // 2.a.
			b1.cnt = 0
			so.EmitTuplesNB(4)
			go func() {
				t.Stop()
			}()
			t.state.Wait(TSStopping)
			b1.EmitTuples(8)
			t.state.Wait(TSStopped)
			checkPostCond()

			Convey("Then the sink should receive all of generated tuples", func() {
				So(len(si.Tuples), ShouldEqual, 4)
			})
		})

		Convey("When generating some tuples and call stop after the sink received some of them", func() { // 2.b.
			b1.cnt = 0
			go func() {
				so.EmitTuplesNB(3)
				b1.EmitTuples(1)
				si.Wait(1)
				go func() {
					t.Stop()
				}()
				t.state.Wait(TSStopping)
				b1.EmitTuples(2)
			}()
			t.state.Wait(TSStopped)
			checkPostCond()

			Convey("Then the sink should receive all of those tuples", func() {
				So(len(si.Tuples), ShouldEqual, 3)
			})
		})

		Convey("When generating some tuples and call stop after the sink received all", func() { // 2.c.
			go func() {
				so.EmitTuples(4)
				si.Wait(4)
				t.Stop()
			}()
			t.state.Wait(TSStopped)
			checkPostCond()

			Convey("Then the sink should only receive those tuples", func() {
				So(len(si.Tuples), ShouldEqual, 4)
			})
		})

		Convey("When generating all tuples and call stop before the sink receives a tuple", func() { // 3.a.
			b1.cnt = 0
			go func() {
				so.EmitTuples(100) // Blocking call. Assuming the pipe's capacity is greater than or equal to 8.
				go func() {
					t.Stop()
				}()
				t.state.Wait(TSStopping)
				b1.EmitTuples(8)
			}()
			t.state.Wait(TSStopped)
			checkPostCond()

			Convey("Then the sink should receive all tuples", func() {
				So(len(si.Tuples), ShouldEqual, 8)
			})
		})

		Convey("When generating all tuples and call stop after the sink received some of them", func() { // 3.b.
			b1.cnt = 2
			go func() {
				so.EmitTuples(100)
				si.Wait(2)
				go func() {
					t.Stop()
				}()
				t.state.Wait(TSStopping)
				b1.EmitTuples(6)
			}()
			t.state.Wait(TSStopped)
			checkPostCond()

			Convey("Then the sink should receive all of those tuples", func() {
				So(len(si.Tuples), ShouldEqual, 8)
			})
		})

		Convey("When generating all tuples and call stop after the sink received all", func() { // 3.c.
			go func() {
				so.EmitTuples(100)
				si.Wait(8)
				t.Stop()
			}()
			t.state.Wait(TSStopped)
			checkPostCond()

			Convey("Then the sink should receive all tuples", func() {
				So(len(si.Tuples), ShouldEqual, 8)
			})
		})

		Convey("When removing the source after generating some tuples", func() {
			Reset(func() {
				t.Stop()
			})
			so.EmitTuples(2)
			So(t.Remove("source"), ShouldBeNil)

			Convey("Then the source should be stopped", func() {
				So(son.State().Get(), ShouldEqual, TSStopped)
			})

			Convey("Then the source shouldn't be found", func() {
				_, err := t.Source("source")
				So(err, ShouldNotBeNil)
			})

			Convey("Then the sink should receive the tuples", func() {
				si.Wait(2)
				So(len(si.Tuples), ShouldEqual, 2)
			})
		})

		Convey("When removing a box after processing some tuples", func() {
			Reset(func() {
				t.Stop()
			})
			so.EmitTuples(2)
			si.Wait(2)
			So(t.Remove("box1"), ShouldBeNil)
			so.EmitTuples(2)

			Convey("Then the box should be stopped", func() {
				So(bn1.State().Get(), ShouldEqual, TSStopped)
			})

			Convey("Then the box shouldn't be found", func() {
				_, err := t.Box("box1")
				So(err, ShouldNotBeNil)
			})

			Convey("Then sink should only receive the processed tuples", func() {
				So(len(si.Tuples), ShouldEqual, 2)
			})

			Convey("And connecting another box in the topology", func() {
				b3 := BoxFunc(forwardBox)
				bn3, err := t.AddBox("box3", b3, nil)
				So(err, ShouldBeNil)
				So(bn3.Input("source", nil), ShouldBeNil)
				So(bn2.Input("box3", nil), ShouldBeNil)
				so.EmitTuples(4)

				Convey("Then the sink should receive the correct number of tuples", func() {
					si.Wait(6) // 2 tuples which send just after box1 was removed were lost.
					So(len(si.Tuples), ShouldEqual, 6)
				})
			})

			Convey("And connecting the sink directly to the source", func() {
				So(sin.Input("source", nil), ShouldBeNil)
				so.EmitTuples(4)

				Convey("Then the sink should receive the correct number of tuples", func() {
					si.Wait(6) // 2 tuples which send just after box1 was removed were lost.
					So(len(si.Tuples), ShouldEqual, 6)
				})
			})
		})

		Convey("When removing a sink after receiving some tuples", func() {
			Reset(func() {
				t.Stop()
			})
			so.EmitTuples(2)
			si.Wait(2)
			So(t.Remove("sink"), ShouldBeNil)
			so.EmitTuples(2)

			Convey("Then the sink should be stopped", func() {
				So(sin.State().Get(), ShouldEqual, TSStopped)
			})

			Convey("Then the sink shouldn't be found", func() {
				_, err := t.Sink("sink")
				So(err, ShouldNotBeNil)
			})

			Convey("Then sink shouldn't receive tuples generated after it got removed", func() {
				So(len(si.Tuples), ShouldEqual, 2)
			})
		})
	})
}

func TestForkDefaultDynamicTopology(t *testing.T) {
	config := Configuration{TupleTraceEnabled: 1}
	ctx := newTestContext(config)

	Convey("Given a simple fork topology", t, func() {
		/*
		 *        /--> b1 -*--> si1
		 *   so -*
		 *        \--> b2 -*--> si2
		 */
		dt := NewDefaultDynamicTopology(ctx, "dt1")
		t := dt.(*defaultDynamicTopology)

		so := NewTupleIncrementalEmitterSource(freshTuples())
		_, err := t.AddSource("source", so, nil)
		So(err, ShouldBeNil)

		b1 := &BlockingForwardBox{cnt: 8}
		tc1 := newTerminateChecker(b1)
		bn1, err := t.AddBox("box1", tc1, nil)
		So(err, ShouldBeNil)
		So(bn1.Input("source", nil), ShouldBeNil)

		b2 := &BlockingForwardBox{cnt: 8}
		tc2 := newTerminateChecker(b2)
		bn2, err := t.AddBox("box2", tc2, nil)
		So(err, ShouldBeNil)
		So(bn2.Input("source", nil), ShouldBeNil)

		si1 := NewTupleCollectorSink()
		sic1 := &sinkCloseChecker{s: si1}
		sin1, err := t.AddSink("si1", sic1, nil)
		So(err, ShouldBeNil)
		So(sin1.Input("box1", nil), ShouldBeNil)

		si2 := NewTupleCollectorSink()
		sic2 := &sinkCloseChecker{s: si2}
		sin2, err := t.AddSink("si2", sic2, nil)
		So(err, ShouldBeNil)
		So(sin2.Input("box2", nil), ShouldBeNil)

		checkPostCond := func() {
			Convey("Then the topology should be stopped", func() {
				So(t.state.Get(), ShouldEqual, TSStopped)
			})

			Convey("Then Box.Terminate should be called exactly once", func() {
				So(tc1.terminateCnt, ShouldEqual, 1)
				So(tc2.terminateCnt, ShouldEqual, 1)
			})

			Convey("Then Sink.Close should be called exactly once", func() {
				So(sic1.closeCnt, ShouldEqual, 1)
				So(sic2.closeCnt, ShouldEqual, 1)
			})
		}

		Convey("When generating no tuples and call stop", func() { // 1.
			go func() {
				t.Stop()
			}()
			t.state.Wait(TSStopped)
			checkPostCond()

			Convey("Then the sink shouldn't receive anything", func() {
				So(si1.Tuples, ShouldBeEmpty)
				So(si2.Tuples, ShouldBeEmpty)
			})
		})

		Convey("When generating some tuples and call stop before the sink receives a tuple", func() { // 2.a.
			b1.cnt = 0 // Block tuples. The box isn't running yet, so cnt can safely be changed.
			go func() {
				so.EmitTuplesNB(4)
				go func() {
					t.Stop()
				}()

				// resume b1 after the topology starts stopping all.
				t.state.Wait(TSStopping)
				b1.EmitTuples(8)
			}()
			t.state.Wait(TSStopped)
			checkPostCond()

			Convey("Then the sink should receive all of generated tuples", func() {
				So(len(si1.Tuples), ShouldEqual, 4)
				So(len(si2.Tuples), ShouldEqual, 4)
			})
		})

		Convey("When generating some tuples and call stop after the sink received some of them", func() { // 2.b.
			go func() {
				so.EmitTuplesNB(3)
				t.state.Wait(TSRunning)
				b1.EmitTuples(1)
				si1.Wait(1)
				go func() {
					t.Stop()
				}()

				t.state.Wait(TSStopping)
				b1.EmitTuples(2)
			}()
			t.state.Wait(TSStopped)
			checkPostCond()

			Convey("Then the sink should receive all of those tuples", func() {
				So(len(si1.Tuples), ShouldEqual, 3)
				So(len(si2.Tuples), ShouldEqual, 3)
			})
		})

		Convey("When generating some tuples and call stop after both sinks received all", func() { // 2.c.
			go func() {
				so.EmitTuples(4)
				si1.Wait(4)
				si2.Wait(4)
				t.Stop()
			}()
			t.state.Wait(TSStopped)
			checkPostCond()

			Convey("Then the sink should only receive those tuples", func() {
				So(len(si1.Tuples), ShouldEqual, 4)
				So(len(si2.Tuples), ShouldEqual, 4)
			})
		})

		Convey("When generating all tuples and call stop before the sink receives a tuple", func() { // 3.a.
			b1.cnt = 0
			go func() {
				so.EmitTuples(100) // Blocking call. Assuming the pipe's capacity is greater than or equal to 8.
				go func() {
					t.Stop()
				}()
				t.state.Wait(TSStopping)
				b1.EmitTuples(8)
			}()
			t.state.Wait(TSStopped)
			checkPostCond()

			Convey("Then the sink should receive all tuples", func() {
				So(len(si1.Tuples), ShouldEqual, 8)
				So(len(si2.Tuples), ShouldEqual, 8)
			})
		})

		Convey("When generating all tuples and call stop after the sink received some of them", func() { // 3.b.
			b1.cnt = 2
			go func() {
				so.EmitTuples(100)
				si1.Wait(2)
				// don't care about si2
				go func() {
					t.Stop()
				}()
				t.state.Wait(TSStopping)
				b1.EmitTuples(6)
			}()
			t.state.Wait(TSStopped)
			checkPostCond()

			Convey("Then the sink should receive all of those tuples", func() {
				So(len(si1.Tuples), ShouldEqual, 8)
				So(len(si2.Tuples), ShouldEqual, 8)
			})
		})

		Convey("When generating all tuples and call stop after the two sinks received some of them", func() { // 3.b'.
			b1.cnt = 2
			b2.cnt = 3
			go func() {
				so.EmitTuples(100)
				si1.Wait(2)
				si2.Wait(3)
				go func() {
					t.Stop()
				}()
				t.state.Wait(TSStopping)
				b1.EmitTuples(6)
				b2.EmitTuples(5)
			}()
			t.state.Wait(TSStopped)
			checkPostCond()

			Convey("Then the sink should receive all of those tuples", func() {
				So(len(si1.Tuples), ShouldEqual, 8)
				So(len(si2.Tuples), ShouldEqual, 8)
			})
		})

		Convey("When generating all tuples and call stop after the sink received all", func() { // 3.c.
			go func() {
				so.EmitTuples(100)
				si1.Wait(8)
				si2.Wait(8)
				t.Stop()
			}()
			t.state.Wait(TSStopped)
			checkPostCond()

			Convey("Then the sink should receive all tuples", func() {
				So(len(si1.Tuples), ShouldEqual, 8)
				So(len(si2.Tuples), ShouldEqual, 8)
			})
		})
	})
}

func TestJoinDefaultDynamicTopology(t *testing.T) {
	config := Configuration{TupleTraceEnabled: 1}
	ctx := newTestContext(config)

	Convey("Given a simple join topology", t, func() {
		/*
		 *   so1 -*-\
		 *           --> b -*--> si
		 *   so2 -*-/
		 */
		tb := NewDefaultDynamicTopology(ctx, "dt1")
		t := tb.(*defaultDynamicTopology)

		so1 := NewTupleIncrementalEmitterSource(freshTuples()[0:4])
		_, err := t.AddSource("source1", so1, nil)
		So(err, ShouldBeNil)

		so2 := NewTupleIncrementalEmitterSource(freshTuples()[4:8])
		_, err = t.AddSource("source2", so2, nil)
		So(err, ShouldBeNil)

		b1 := &BlockingForwardBox{cnt: 8}
		tc1 := newTerminateChecker(b1)
		bn1, err := t.AddBox("box1", tc1, nil)
		So(err, ShouldBeNil)
		So(bn1.Input("source1", nil), ShouldBeNil)
		So(bn1.Input("source2", nil), ShouldBeNil)

		si := NewTupleCollectorSink()
		sic := &sinkCloseChecker{s: si}
		sin, err := t.AddSink("sink", sic, nil)
		So(err, ShouldBeNil)
		So(sin.Input("box1", nil), ShouldBeNil)

		checkPostCond := func() {
			Convey("Then the topology should be stopped", func() {
				So(t.state.Get(), ShouldEqual, TSStopped)
			})

			Convey("Then Box.Terminate should be called exactly once", func() {
				So(tc1.terminateCnt, ShouldEqual, 1)
			})

			Convey("Then Sink.Close should be called exactly once", func() {
				So(sic.closeCnt, ShouldEqual, 1)
			})
		}

		Convey("When generating no tuples and call stop", func() { // 1.
			go func() {
				t.Stop()
			}()
			t.state.Wait(TSStopped)
			checkPostCond()

			Convey("Then the sink shouldn't receive anything", func() {
				So(si.Tuples, ShouldBeEmpty)
			})
		})

		Convey("When generating some tuples and call stop before the sink receives a tuple", func() { // 2.a.
			b1.cnt = 0 // Block tuples. The box isn't running yet, so cnt can safely be changed.
			go func() {
				so1.EmitTuplesNB(3)
				so2.EmitTuplesNB(1)
				go func() {
					t.Stop()
				}()

				// resume b1 after the topology starts stopping all.
				t.state.Wait(TSStopping)
				b1.EmitTuples(8)
			}()
			t.state.Wait(TSStopped)
			checkPostCond()

			Convey("Then the sink should receive all of generated tuples", func() {
				So(len(si.Tuples), ShouldEqual, 4)
			})
		})

		Convey("When generating some tuples from one source and call stop before the sink receives a tuple", func() { // 2.a'.
			b1.cnt = 0 // Block tuples. The box isn't running yet, so cnt can safely be changed.
			go func() {
				so1.EmitTuplesNB(3)
				go func() {
					t.Stop()
				}()

				// resume b1 after the topology starts stopping all.
				t.state.Wait(TSStopping)
				b1.EmitTuples(8)
			}()
			t.state.Wait(TSStopped)
			checkPostCond()

			Convey("Then the sink should receive all of generated tuples", func() {
				So(len(si.Tuples), ShouldEqual, 3)
			})
		})

		Convey("When generating some tuples and call stop after the sink received some of them", func() { // 2.b.
			go func() {
				so1.EmitTuplesNB(1)
				so2.EmitTuplesNB(2)
				t.state.Wait(TSRunning)
				b1.EmitTuples(1)
				si.Wait(1)
				go func() {
					t.Stop()
				}()

				t.state.Wait(TSStopping)
				b1.EmitTuples(2)
			}()
			t.state.Wait(TSStopped)
			checkPostCond()

			Convey("Then the sink should receive all of those tuples", func() {
				So(len(si.Tuples), ShouldEqual, 3)
			})
		})

		Convey("When generating some tuples and call stop after the sink received all", func() { // 2.c.
			go func() {
				so1.EmitTuples(2)
				so2.EmitTuples(1)
				si.Wait(3)
				t.Stop()
			}()
			t.state.Wait(TSStopped)
			checkPostCond()

			Convey("Then the sink should only receive those tuples", func() {
				So(len(si.Tuples), ShouldEqual, 3)
			})
		})

		Convey("When generating all tuples and call stop before the sink receives a tuple", func() { // 3.a.
			b1.cnt = 0
			go func() {
				so1.EmitTuples(100) // Blocking call. Assuming the pipe's capacity is greater than or equal to 8.
				so2.EmitTuples(100)
				go func() {
					t.Stop()
				}()
				t.state.Wait(TSStopping)
				b1.EmitTuples(8)
			}()
			t.state.Wait(TSStopped)
			checkPostCond()

			Convey("Then the sink should receive all tuples", func() {
				So(len(si.Tuples), ShouldEqual, 8)
			})
		})

		Convey("When generating all tuples and call stop after the sink received some of them", func() { // 3.b.
			b1.cnt = 2
			go func() {
				so1.EmitTuples(100)
				so2.EmitTuples(100)
				si.Wait(2)
				go func() {
					t.Stop()
				}()
				t.state.Wait(TSStopping)
				b1.EmitTuples(6)
			}()
			t.state.Wait(TSStopped)
			checkPostCond()

			Convey("Then the sink should receive all of those tuples", func() {
				So(len(si.Tuples), ShouldEqual, 8)
			})
		})

		Convey("When generating all tuples and call stop after the sink received all", func() { // 3.c.
			go func() {
				so1.EmitTuples(100)
				so2.EmitTuples(100)
				si.Wait(8)
				t.Stop()
			}()
			t.state.Wait(TSStopped)
			checkPostCond()

			Convey("Then the sink should receive all tuples", func() {
				So(len(si.Tuples), ShouldEqual, 8)
			})
		})
	})
}

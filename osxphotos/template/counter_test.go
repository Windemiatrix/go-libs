package template

import (
	"cmp"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

type counterStep struct {
	before func(*Renderer)
	in     string
	want   []string
	err    error
}

func renderOn(t *testing.T, r *Renderer, in string) ([]string, error) {
	t.Helper()
	tmpl, err := Parse(in)
	if err != nil {
		t.Fatalf("Parse(%q): %v", in, err)
	}
	got, _, err := r.Render(tmpl, fakePhoto{"keyword": strs("a", "b")}, RenderOptions{})
	return got, err
}

// get_counter_value, reset_all_counters and reset_counter (template_counter.py:52-114).
func TestCounter(t *testing.T) {
	t.Parallel()
	one := func(in, want string) counterStep { return counterStep{in: in, want: []string{want}} }
	fails := func(in string, err error) counterStep { return counterStep{in: in, err: err} }
	reset := func(field string) func(*Renderer) { return func(r *Renderer) { r.ResetCounter(field) } }
	resetAll := func(r *Renderer) { r.ResetCounters() }
	for _, tt := range []struct {
		name  string
		steps []counterStep
	}{
		{"grows from render to render", []counterStep{one("{counter}", "0"), one("{counter}", "1"), one("{counter}", "2")}},
		{"two fields of a template", []counterStep{one("{counter}{counter}", "01"), one("{counter}", "2")}},
		{"one step for every combination", []counterStep{{in: "{counter}-{keyword}", want: []string{"0-a", "0-b"}}, one("{counter}", "1")}},
		{"start", []counterStep{one("{counter(1)}", "1"), one("{counter(1)}", "2")}},
		{"format", []counterStep{one("{counter:05d(1)}", "00001"), one("{counter:05d(1)}", "00002")}},
		{"format of a negative value", []counterStep{one("{counter:03d(-1)}", "-01"), one("{counter:03d(-1)}", "000")}},
		{"stop is not reached", []counterStep{
			one("{counter(1,3)}", "1"), one("{counter(1,3)}", "2"), one("{counter(1,3)}", "1"), one("{counter(1,3)}", "2"),
		}},
		{"stop at the start", []counterStep{one("{counter(2,2)}", "2"), one("{counter(2,2)}", "2")}},
		// the count after a stop is step, not start (90)
		{"stop with a step", []counterStep{
			one("{counter(1,6,2)}", "1"), one("{counter(1,6,2)}", "3"), one("{counter(1,6,2)}", "5"),
			one("{counter(1,6,2)}", "1"), one("{counter(1,6,2)}", "3"), one("{counter(1,6,2)}", "5"),
		}},
		{"negative stop", []counterStep{
			one("{counter(-3,-1)}", "-3"), one("{counter(-3,-1)}", "-2"), one("{counter(-3,-1)}", "-3"), one("{counter(-3,-1)}", "-2"),
		}},
		{"step", []counterStep{one("{counter(,,2)}", "0"), one("{counter(,,2)}", "2"), one("{counter(,,2)}", "4")}},
		{"negative step", []counterStep{one("{counter(5,,-2)}", "5"), one("{counter(5,,-2)}", "3"), one("{counter(5,,-2)}", "1")}},
		{"zero step", []counterStep{one("{counter(5,,0)}", "5"), one("{counter(5,,0)}", "5")}},
		// int() strips the spaces of a number (65-67)
		{"spaces around numbers", []counterStep{one("{counter( 1 ,, 2 )}", "1"), one("{counter( 1 ,, 2 )}", "3")}},
		{"a tab and a newline around a number", []counterStep{one("{counter(\t1\n)}", "1")}},
		// upstream renders combine, bool and default before the field (phototemplate.py:577-622, 658)
		{"parts render before the field", []counterStep{one("{counter?{counter},{counter}}", "0"), one("{counter}", "3")}},
		{"combine renders first", []counterStep{{in: "{counter&{counter}}", want: []string{"1", "0"}}}},
		{"default is rendered eagerly", []counterStep{{in: "{keyword,{counter}}", want: []string{"a", "b"}}, one("{counter}", "1")}},
		{"in a condition", []counterStep{one("{counter == 1?Y,N}", "N"), one("{counter == 1?Y,N}", "Y")}},
		{"named counters are separate", []counterStep{
			one("{counter.a}", "0"), one("{counter.a}", "1"), one("{counter}", "0"), one("{counter.}", "0"), one("{counter.a}", "2"),
		}},
		// the settings compare as numbers (76-82)
		{"same settings written otherwise", []counterStep{
			one("{counter}", "0"), one("{counter(0,0,1)}", "1"), one("{counter(,,)}", "2"), one("{counter()}", "3"), one("{counter:d}", "4"),
		}},
		{"settings cannot change", []counterStep{one("{counter(1)}", "1"), fails("{counter(2)}", ErrRender), one("{counter(1)}", "2")}},
		{"named settings cannot change", []counterStep{one("{counter.a(,,2)}", "0"), fails("{counter.a}", ErrRender), one("{counter}", "0")}},
		// upstream formats after the count has grown (86, 92-93)
		{"a bad format still counts", []counterStep{fails("{counter:.2d}", ErrRender), one("{counter}", "1")}},
		{"a wide format", []counterStep{fails("{counter:999999999}", ErrTooManyValues)}},
		// int() raises ValueError, which upstream does not catch (63-71)
		{"stop below start", []counterStep{fails("{counter(3,1)}", ErrRender), one("{counter(3)}", "3")}},
		{"negative stop below start", []counterStep{fails("{counter(-1,-3)}", ErrRender)}},
		{"not a number", []counterStep{fails("{counter(x)}", ErrRender)}},
		{"a decimal", []counterStep{fails("{counter(1.5)}", ErrRender)}},
		{"only spaces", []counterStep{fails("{counter( )}", ErrRender)}},
		{"a space between commas", []counterStep{fails("{counter( 1 , , 2 )}", ErrRender)}},
		{"four arguments", []counterStep{fails("{counter(1,2,3,4)}", ErrRender), fails("{counter(,,,)}", ErrRender)}},
		// int() takes it; UPSTREAM.md has what strconv.ParseInt does not take, as for slice
		{"an underscore", []counterStep{fails("{counter(1_0)}", ErrRender)}},
		// the int of Python has no bound; UPSTREAM.md has the int64 bound
		{"beyond int64", []counterStep{fails("{counter(9223372036854775808)}", ErrRender)}},
		{"count beyond int64", []counterStep{
			one("{counter(9223372036854775807)}", "9223372036854775807"), fails("{counter(9223372036854775807)}", ErrRender),
		}},
		// Python gives 9223372036854775806 here, but the count after it is past int64
		{"step beyond int64", []counterStep{
			one("{counter(-1,,9223372036854775807)}", "-1"), fails("{counter(-1,,9223372036854775807)}", ErrRender),
			fails("{counter(-1,,9223372036854775807)}", ErrRender),
		}},
		{"count below int64", []counterStep{
			one("{counter(-9223372036854775808,,-1)}", "-9223372036854775808"), fails("{counter(-9223372036854775808,,-1)}", ErrRender),
		}},
		{"step below int64", []counterStep{
			one("{counter(0,,-9223372036854775808)}", "0"), fails("{counter(0,,-9223372036854775808)}", ErrRender),
		}},
		// Python resets to 1 here (87-90), but 1 + step is past int64
		{"stop with a step beyond int64", []counterStep{
			one("{counter(1,10,9223372036854775807)}", "1"), fails("{counter(1,10,9223372036854775807)}", ErrRender),
		}},
		// reset_all_counters and reset_counter (105-114)
		{"ResetCounters", []counterStep{
			one("{counter}", "0"), one("{counter.a}", "0"), one("{counter}", "1"),
			{before: resetAll, in: "{counter}", want: []string{"0"}}, one("{counter.a}", "0"),
		}},
		{"ResetCounters forgets the settings", []counterStep{one("{counter(1)}", "1"), {before: resetAll, in: "{counter(5)}", want: []string{"5"}}}},
		{"ResetCounter sets the count to the start", []counterStep{
			one("{counter(5)}", "5"), one("{counter(5)}", "6"), {before: reset("counter"), in: "{counter(5)}", want: []string{"10"}},
		}},
		{"ResetCounter of one counter", []counterStep{
			one("{counter}", "0"), one("{counter}", "1"), one("{counter.a}", "0"),
			{before: reset("counter"), in: "{counter}", want: []string{"0"}}, one("{counter.a}", "1"),
		}},
		{"ResetCounter keeps the settings", []counterStep{
			one("{counter(1)}", "1"), {before: reset("counter"), in: "{counter}", err: ErrRender},
		}},
		{"ResetCounter of an unknown field", []counterStep{
			one("{counter}", "0"), {before: reset("nope"), in: "{counter}", want: []string{"1"}},
		}},
		{"ResetCounters of a new Renderer", []counterStep{{before: resetAll, in: "{counter}", want: []string{"0"}}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := NewRenderer()
			for i, step := range tt.steps {
				if step.before != nil {
					step.before(r)
				}
				got, err := renderOn(t, r, step.in)
				if !errors.Is(err, step.err) || !slices.Equal(got, step.want) || err != nil && !strings.Contains(err.Error(), "counter") {
					t.Fatalf("step %d: Render(%q) = %q, %v; want %q, %v naming the counter", i+1, step.in, got, err, step.want, step.err)
				}
			}
		})
	}
}

func TestCounterPerRenderer(t *testing.T) {
	t.Parallel()
	a, b := NewRenderer(), NewRenderer()
	for _, tt := range []struct {
		r    *Renderer
		want string
	}{{a, "0"}, {a, "1"}, {b, "0"}, {a, "2"}, {&Renderer{}, "0"}} {
		if got, err := renderOn(t, tt.r, "{counter}"); err != nil || !slices.Equal(got, []string{tt.want}) {
			t.Errorf("Render({counter}) = %q, %v; want %q", got, err, tt.want)
		}
	}
}

// ResetCounter of a counter that never exists keeps the values exact;
// TestCounterResetRace resets counters that exist.
func TestCounterRace(t *testing.T) {
	t.Parallel()
	tmpl, err := Parse("{counter}")
	if err != nil {
		t.Fatal(err)
	}
	r := NewRenderer()
	var (
		mu  sync.Mutex
		got []int
		wg  sync.WaitGroup
	)
	for range 8 {
		wg.Go(func() {
			for range 100 {
				vals, _, err := r.Render(tmpl, fakePhoto{}, RenderOptions{})
				n, errAtoi := strconv.Atoi(strings.Join(vals, "|"))
				if err = cmp.Or(err, errAtoi); err != nil {
					t.Error(err)
					return
				}
				mu.Lock()
				got = append(got, n)
				mu.Unlock()
			}
		})
	}
	wg.Go(func() {
		for range 100 {
			r.ResetCounter("counter.b")
		}
	})
	wg.Wait()
	slices.Sort(got)
	want := make([]int, 800)
	for i := range want {
		want[i] = i
	}
	if !slices.Equal(got, want) {
		t.Errorf("8 goroutines of 100 renders of {counter} gave %d values, not 0 to 799 once each", len(got))
	}
}

// The values depend on when a reset runs, so -race does the checking.
func TestCounterResetRace(t *testing.T) {
	t.Parallel()
	tmpl, err := Parse("{counter}{counter.a}")
	if err != nil {
		t.Fatal(err)
	}
	r := NewRenderer()
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 100 {
				if _, _, err := r.Render(tmpl, fakePhoto{}, RenderOptions{}); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	wg.Go(func() {
		for range 100 {
			r.ResetCounters()
		}
	})
	wg.Go(func() {
		for range 100 {
			r.ResetCounter("counter")
			r.ResetCounter("counter.a")
		}
	})
	wg.Wait()
}

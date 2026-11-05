package template

import (
	"fmt"
	"strconv"
	"strings"
)

// counterSettings and counter are CounterSettings and Counter (template_counter.py:18-24).
type counterSettings struct{ start, stop, step int64 }

type counter struct {
	settings counterSettings
	count    int64
}

// ResetCounters forgets every counter: the next {counter} starts again
// (reset_all_counters, template_counter.py:105-108).
func (r *Renderer) ResetCounters() {
	r.mu.Lock()
	defer r.mu.Unlock()
	clear(r.counters)
}

// ResetCounter sets the count of the counter named field ("counter",
// "counter.a") back to its start value, as reset_counter does
// (template_counter.py:111-114), so the next value is start + start, or start
// when that reaches stop. An unknown field does nothing.
func (r *Renderer) ResetCounter(field string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.counters[field]; ok {
		c.count = c.settings.start
	}
}

// counterValue is get_counter_value (template_counter.py:52-94); the state of
// a counter is kept by the whole name of its field, "counter.a" (77).
func (s *rendering) counterValue(f *Field) (string, error) {
	var arg string
	if f.Arg != nil {
		arg = *f.Arg
	}
	set := counterSettings{step: 1}
	dst := [...]*int64{&set.start, &set.stop, &set.step}
	parts := strings.SplitN(arg, ",", len(dst)) // a fourth argument fails in strconv.ParseInt, as unpacking three raises ValueError (63)
	for i, p := range parts {
		if p == "" { // 65-67; int(" ") raises ValueError, which upstream does not catch (68)
			continue
		}
		n, err := strconv.ParseInt(strings.TrimFunc(p, isSpace), 10, 64)
		if err != nil {
			return "", fmt.Errorf("%w: %s takes (start,stop,step) as integers within int64", ErrRender, f.Name)
		}
		*dst[i] = n
	}
	if set.stop != 0 && set.stop < set.start { // 73-74
		return "", fmt.Errorf("%w: %s stops before its start", ErrRender, f.Name)
	}
	n, err := s.r.next(f.Name, set)
	if err != nil {
		return "", err
	}
	out, err := s.formatValue(n, f.Subfield) // 92-93, after the count has grown
	return out, inField(err, f)
}

// next is template_counter.py:76-90 under the lock: a Renderer serves several goroutines.
func (r *Renderer) next(field string, set counterSettings) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.counters[field]
	if !ok {
		if r.counters == nil { // the zero Renderer is ready to use
			r.counters = map[string]*counter{}
		}
		c = &counter{settings: set}
		r.counters[field] = c
	} else if c.settings != set { // 79-82
		return 0, fmt.Errorf("%w: the arguments of %s cannot change", ErrRender, field)
	}
	value, count := set.start+c.count, c.count+set.step
	if (value < set.start) != (c.count < 0) || (count < c.count) != (set.step < 0) { // the int of Python has no bound
		return 0, fmt.Errorf("%w: %s counts past int64", ErrRender, field)
	}
	c.count = count
	if set.stop != 0 && value >= set.stop { // 87-90
		value, c.count = set.start, set.step
	}
	return value, nil
}

package logs

type Ring[T any] struct {
	capacity int
	values   []T
}

func NewRing[T any](capacity int) *Ring[T] {
	if capacity < 1 {
		capacity = 1
	}
	return &Ring[T]{capacity: capacity, values: make([]T, 0, capacity)}
}

func (r *Ring[T]) Add(value T) {
	if len(r.values) == r.capacity {
		copy(r.values, r.values[1:])
		r.values[len(r.values)-1] = value
		return
	}
	r.values = append(r.values, value)
}

func (r *Ring[T]) Values() []T { return append([]T{}, r.values...) }
func (r *Ring[T]) Clear()      { r.values = r.values[:0] }

package test

// Updater allows to pass an object and get a changed copy of it in a single line.
// Useful during tests for quick object manipulation.
/* Example:
type changeme struct {
	Field string
}

func main() {
	c := changeme{}
	NewUpdater(c).Update(func(src *changeme) {
		c.Field = "test"
	}).Get()
}
*/

type Updater[T any] struct {
	data *T
}

func NewUpdater[T any](src T) *Updater[T] {
	return &Updater[T]{data: &src}
}

func (m *Updater[T]) Update(transformer func(src *T)) *Updater[T] {
	transformer(m.data)

	return m
}

func (m *Updater[T]) Get() T {
	return *m.data
}

func (m *Updater[T]) GetPtr() *T {
	return m.data
}

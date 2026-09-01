package deps

import (
	"fmt"

	"omastack/internal/model"
)

type CycleError struct{ Path []string }

func (e *CycleError) Error() string { return fmt.Sprintf("dependency cycle: %v", e.Path) }

func StartupOrder(services map[string]model.Service, targets []string) ([]string, error) {
	state := make(map[string]uint8, len(services))
	order := make([]string, 0, len(services))
	stack := make([]string, 0, len(services))

	var visit func(string) error
	visit = func(id string) error {
		svc, ok := services[id]
		if !ok {
			return fmt.Errorf("unknown service %q", id)
		}
		switch state[id] {
		case 2:
			return nil
		case 1:
			start := 0
			for start < len(stack) && stack[start] != id {
				start++
			}
			path := append([]string{}, stack[start:]...)
			path = append(path, id)
			return &CycleError{Path: path}
		}
		state[id] = 1
		stack = append(stack, id)
		for _, dependency := range svc.Dependencies {
			if err := visit(dependency.ServiceID); err != nil {
				return err
			}
		}
		stack = stack[:len(stack)-1]
		state[id] = 2
		order = append(order, id)
		return nil
	}

	for _, id := range targets {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	return order, nil
}

func ShutdownOrder(services map[string]model.Service, targets []string) ([]string, error) {
	order, err := StartupOrder(services, targets)
	if err != nil {
		return nil, err
	}
	for left, right := 0, len(order)-1; left < right; left, right = left+1, right-1 {
		order[left], order[right] = order[right], order[left]
	}
	return order, nil
}

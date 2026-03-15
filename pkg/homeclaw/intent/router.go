package intent

// Router maps IntentTypes to Intent handlers and dispatches incoming
// IntentResults to the appropriate handler.
type Router struct {
	handlers map[IntentType]Intent
}

// NewRouter creates a Router pre-populated with the provided Intent handlers.
// Each handler is registered under all types returned by its Types() method.
// If multiple handlers share a type, the last one wins.
func NewRouter(intents ...Intent) *Router {
	r := &Router{
		handlers: make(map[IntentType]Intent),
	}
	for _, h := range intents {
		for _, t := range h.Types() {
			r.handlers[t] = h
		}
	}
	return r
}

// Register adds or replaces the handler for all types returned by h.Types().
func (r *Router) Register(h Intent) {
	for _, t := range h.Types() {
		r.handlers[t] = h
	}
}

// Route looks up the handler for result.Type.
// Returns (handler, true) when a handler is registered, or (nil, false) when
// no handler exists — the caller should then fall through to the large model.
func (r *Router) Route(result IntentResult) (Intent, bool) {
	h, ok := r.handlers[result.Type]
	return h, ok
}

// HandledTypes returns all IntentTypes that have registered handlers.
func (r *Router) HandledTypes() []IntentType {
	types := make([]IntentType, 0, len(r.handlers))
	for t := range r.handlers {
		types = append(types, t)
	}
	return types
}

// Shared function-call reassembly for Responses-style SSE streams.
// Multiple providers (codex first, claude/antigravity next) correlate
// argument deltas by item_id while the stable tool-call id is the call_id.
// Deltas or done events arriving before output_item.added are buffered and
// attached once the call appears — mirroring the TS stream-fn logic.
package shared

// Call is one reassembled function call.
type Call struct {
	ItemID    string
	CallID    string
	Name      string
	Arguments string
	Finalized bool
	Emitted   bool
}

type pendingFinal struct {
	name      string
	arguments string
	hasName   bool
}

// Accumulator reassembles function calls across out-of-order SSE events.
type Accumulator struct {
	byItem  map[string]*Call
	byCall  map[string]*Call
	order   []string
	deltas  map[string]string
	pending map[string]pendingFinal
}

// NewAccumulator creates an empty accumulator.
func NewAccumulator() *Accumulator {
	return &Accumulator{
		byItem:  map[string]*Call{},
		byCall:  map[string]*Call{},
		deltas:  map[string]string{},
		pending: map[string]pendingFinal{},
	}
}

// Add registers output_item.added. Returns the call when a buffered done
// event already finalized it.
func (a *Accumulator) Add(itemID, callID, name, args string) *Call {
	c := &Call{ItemID: itemID, CallID: callID, Name: name, Arguments: args}
	if buffered, ok := a.deltas[itemID]; ok {
		c.Arguments += buffered
		delete(a.deltas, itemID)
	}
	if buffered, ok := a.pending[itemID]; ok {
		if buffered.hasName {
			c.Name = buffered.name
		}
		c.Arguments = buffered.arguments
		c.Finalized = true
		delete(a.pending, itemID)
	}
	a.byItem[itemID] = c
	a.byCall[callID] = c
	a.order = append(a.order, callID)
	if c.Finalized {
		return c
	}
	return nil
}

// Delta appends an argument fragment for itemID.
func (a *Accumulator) Delta(itemID, fragment string) {
	if c, ok := a.byItem[itemID]; ok {
		c.Arguments += fragment
	} else if itemID != "" {
		a.deltas[itemID] += fragment
	}
}

// Done records function_call_arguments.done. Returns the finalized call when
// its added event was already seen; otherwise buffers for a later Add.
func (a *Accumulator) Done(itemID, name, args string) *Call {
	if c, ok := a.byItem[itemID]; ok {
		if name != "" {
			c.Name = name
		}
		c.Arguments = args
		c.Finalized = true
		return c
	}
	if itemID != "" {
		a.pending[itemID] = pendingFinal{name: name, arguments: args, hasName: name != ""}
	}
	return nil
}

// Unfinalized returns calls that never received done, in arrival order.
func (a *Accumulator) Unfinalized() []*Call {
	var out []*Call
	for _, callID := range a.order {
		if c, ok := a.byCall[callID]; ok && !c.Finalized {
			out = append(out, c)
		}
	}
	return out
}

package vsphere

import (
	"context"
	"errors"
	"fmt"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/event"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
)

// A list of known event IDs that we use for querying events.
const (
	eventTypeCustomizationSucceeded = "CustomizationSucceeded"
)

// virtualMachineCustomizationWaiter is an object that waits for customization
// of a VirtualMachine to complete, by watching for success or failure events.
//
// The waiter should be created with newWaiter **before** the start of the
// customization task to be 100% certain that completion events are not missed.
type virtualMachineCustomizationWaiter struct {
	// This channel will be closed upon completion, and should be blocked on.
	done chan struct{}

	// Any error received from the waiter - be it the customization failure
	// itself, timeouts waiting for the completion events, or other API-related
	// errors. This will always be nil until done is closed.
	err error
}

// Done returns the done channel. This channel will be closed upon completion,
// and should be blocked on.
func (w *virtualMachineCustomizationWaiter) Done() chan struct{} {
	return w.done
}

// Err returns any error received from the waiter. This will always be nil
// until the channel returned by Done is closed.
func (w *virtualMachineCustomizationWaiter) Err() error {
	return w.err
}

// newVirtualMachineCustomizationWaiter returns a new
// virtualMachineCustomizationWaiter to use to wait for customization on.
//
// This should be called **before** the start of the customization task to be
// 100% certain that completion events are not missed.
func newVirtualMachineCustomizationWaiter(client *govmomi.Client, vm *object.VirtualMachine) *virtualMachineCustomizationWaiter {
	w := &virtualMachineCustomizationWaiter{
		done: make(chan struct{}),
	}
	go func() {
		w.err = w.wait(client, vm)
		close(w.done)
	}()
	return w
}

// wait waits for the customization of a supplied VirtualMachine to complete,
// either due to success or error. It does this by watching specifically for
// CustomizationSucceeded and CustomizationFailed events. If the customization
// failed due to some sort of error, the full formatted message is returned as
// an error.
func (w *virtualMachineCustomizationWaiter) wait(client *govmomi.Client, vm *object.VirtualMachine) error {
	// Our listener loop callback.
	success := make(chan struct{})
	cb := func(obj types.ManagedObjectReference, page []types.BaseEvent) error {
		for _, be := range page {
			switch e := be.(type) {
			case *types.CustomizationFailed:
				return errors.New(e.GetEvent().FullFormattedMessage)
			case *types.CustomizationSucceeded:
				close(success)
			}
		}
		return nil
	}

	mgr := event.NewManager(client.Client)
	mgrErr := make(chan error, 1)
	// Make a proper background context so that we can gracefully cancel the
	// subscriber when we are done with it. This eventually gets passed down to
	// the property collector SOAP calls.
	pctx, pcancel := context.WithCancel(context.Background())
	defer pcancel()
	go func() {
		mgrErr <- mgr.Events(pctx, []types.ManagedObjectReference{vm.Reference()}, 10, true, false, cb)
	}()

	// This is our waiter. We want to wait on all of these conditions. We also
	// use a different context so that we can give a better error message on
	// timeout without interfering with the subscriber's context.
	ctx, cancel := context.WithTimeout(context.Background(), defaultAPITimeout)
	defer cancel()
	select {
	case err := <-mgrErr:
		return err
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("timeout waiting for customization to complete")
		}
	case <-success:
		// Pass case to break to success
	}
	return nil
}

// selectEventsForReference allows you to query events for a specific
// ManagedObjectReference.
//
// Event types can be supplied to this function via the eventTypes parameter.
// This is highly recommended when you expect the list of events to be large,
// as there is no limit on returned events.
func selectEventsForReference(client *govmomi.Client, ref types.ManagedObjectReference, eventTypes []string) ([]types.BaseEvent, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultAPITimeout)
	defer cancel()
	filter := types.EventFilterSpec{
		Entity: &types.EventFilterSpecByEntity{
			Entity:    ref,
			Recursion: types.EventFilterSpecRecursionOptionAll,
		},
		EventTypeId: eventTypes,
	}
	mgr := event.NewManager(client.Client)
	return mgr.QueryEvents(ctx, filter)
}

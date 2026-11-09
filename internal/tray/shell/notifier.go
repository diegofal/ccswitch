package shell

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// Notifier posts desktop notifications through Wails' notifications
// service. It is the core's ports.Notifier in the tray, and must be passed
// to Run, which registers the service with the app.
type Notifier struct {
	ns   *notifications.NotificationService
	next atomic.Int64
}

// NewNotifier returns the app's notifier.
func NewNotifier() *Notifier {
	n := &Notifier{ns: notifications.New()}
	n.next.Store(time.Now().UnixNano())
	return n
}

// Notify posts one notification; each gets its own ID so none replaces an
// earlier one still on screen.
func (n *Notifier) Notify(title, body string) error {
	id := fmt.Sprintf("ccswitch-%d", n.next.Add(1))
	if err := n.ns.SendNotification(notifications.NotificationOptions{ID: id, Title: title, Body: body}); err != nil {
		return fmt.Errorf("notification %q: %w", title, err)
	}
	return nil
}

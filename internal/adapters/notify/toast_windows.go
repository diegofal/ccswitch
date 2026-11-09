package notify

import (
	"sync"

	toastlib "git.sr.ht/~jackmordaunt/go-toast/v2"
)

var (
	registerOnce sync.Once
	errRegister  error
)

// toast posts a Windows toast through the WinRT API. Windows shows toasts
// only for an app it knows, so the first one registers AppID under
// HKCU\Software\Classes\AppUserModelId (no admin needed). If the API fails,
// go-toast falls back to PowerShell with a hidden window and returns both
// errors.
func toast(title, body string) error {
	registerOnce.Do(func() { errRegister = toastlib.SetAppData(toastlib.AppData{AppID: AppID}) })
	if errRegister != nil {
		return errRegister
	}
	n := toastlib.Notification{AppID: AppID, Title: title, Body: body}
	return n.Push()
}

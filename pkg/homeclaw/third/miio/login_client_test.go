package miio

import (
	"errors"
	"testing"
)

// p is shared between TestLogin and TestLogin_CompleteTwoFactor so that
// the same session (cookies, ick, notifURL) is preserved across the two calls.
var sharedConnector = NewPasswordConnector("17091616150", "52111125lili")

func TestLogin(t *testing.T) {
	result, err := sharedConnector.Login()
	if err != nil {
		var tfa *ErrTwoFactorRequired
		if errors.As(err, &tfa) {
			t.Logf("2FA required, context: %s", tfa.TwoFactorContext)
			t.Log("Run TestLogin_CompleteTwoFactor with the email code to finish login")
			return
		}
		t.Fatalf("login failed: %v", err)
	}
	t.Logf("login result: %+v", result)
}

// TestLogin_CompleteTwoFactor finishes the login after a 2FA email is received.
// Steps:
//  1. Run TestLogin first — it triggers the email and logs the context.
//  2. Set emailCode below to the code received in the email.
//  3. Run only this test: go test -run "TestLogin_CompleteTwoFactor" -v -timeout 60s
func TestLogin_CompleteTwoFactor(t *testing.T) {
	// First trigger login to send the email (re-uses sharedConnector session)
	emailCode := "755580"
	if emailCode == "" {
		t.Skip("set emailCode to the verification code from your email, then re-run")
	}

	result, err := sharedConnector.CompleteTwoFactor(emailCode)
	if err != nil {
		t.Fatalf("CompleteTwoFactor failed: %v", err)
	}
	t.Logf("login result: %+v", result)
}

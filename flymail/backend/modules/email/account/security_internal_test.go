package account

import (
	"testing"

	"flymail-core/types"
)

func TestParseSecurity(t *testing.T) {
	if parseSecurity("ssl") != types.SecuritySSL {
		t.Error("ssl")
	}
	if parseSecurity("starttls") != types.SecurityStartTLS {
		t.Error("starttls")
	}
	if parseSecurity("") != types.SecurityNone {
		t.Error("空应回退 none")
	}
}

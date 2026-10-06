package gitctx

import (
	"strings"
	"testing"
)

func TestSSHHostname_RefusesOptionLikeHosts(t *testing.T) {
	for _, alias := range []string{"-oProxyCommand=touch /tmp/pwned", "-G", ""} {
		_, err := sshHostname(alias)
		if err == nil || !strings.Contains(err.Error(), "invalid ssh host") {
			t.Errorf("sshHostname(%q) err = %v", alias, err)
		}
	}
}

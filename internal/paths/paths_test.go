package paths

import "testing"

func TestAllowedDeniesSensitivePrefixes(t *testing.T) {
	denied := []string{"/", "/etc", "/etc/nginx", "/root", "/var/log", "/proc/1", "/sys", "/dev/null"}
	for _, p := range denied {
		if Allowed(p, "") {
			t.Errorf("expected deny %s", p)
		}
	}
}

func TestAllowedHomeAndUsrLocal(t *testing.T) {
	ok := []string{"/home/karutoil", "/home/karutoil/src", "/usr/local/src", "/opt/apps", "/tmp/work"}
	for _, p := range ok {
		if !Allowed(p, "") {
			t.Errorf("expected allow %s", p)
		}
	}
}

func TestAllowRoot(t *testing.T) {
	if !Allowed("/home/karutoil/app", "/home/karutoil") {
		t.Fatal("child of allow-root should pass")
	}
	if Allowed("/home/other/app", "/home/karutoil") {
		t.Fatal("outside allow-root must fail")
	}
	if !Allowed("/home/karutoil", "/home/karutoil") {
		t.Fatal("allow-root itself should pass")
	}
}

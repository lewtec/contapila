package filesys

import "testing"

func TestOverlayReadFile(t *testing.T) {
	o := NewOverlay(OS{})
	o.Set("/tmp/foo.beancount", "hello")
	b, err := o.ReadFile("/tmp/foo.beancount")
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "hello" {
		t.Fatalf("%q", b)
	}
}

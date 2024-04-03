package transport_test

import (
	"flag"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/gliderlabs/ssh"

	"github.com/scrapli/scrapligo/util"
)

var (
	update = flag.Bool( //nolint
		"update",
		false,
		"update the golden files",
	)
	functional = flag.Bool( //nolint
		"functional",
		false,
		"execute functional tests",
	)
	platforms = flag.String( //nolint
		"platforms",
		util.All,
		"comma sep list of platform(s) to target",
	)
	transports = flag.String( //nolint
		"transports",
		util.All,
		"comma sep list of transport(s) to target",
	)
)

type DummyServer struct {
	s *ssh.Server
	block chan(struct{})
}
func dummyDevice(t *testing.T)  *DummyServer {
	// base on following logs
	// server that mimics the locking process (every host will lock)
	// read: b"Warning: Permanently added '########' (ECDSA) to the list of known hosts.\n"
	// read: b'######            |\n*------------------------------------------------------------------------------*\n'
	// read: b"#####\n"
	// read: b'######.\n'
	// read: b'#####'
	// write: REDACTED
	// write: '\n'
	// read: b'\n'
	// read: b'\n\ndevice banner\n\n\n'
	// read: b'Password: \n'
	// write: REDACTED
	// write: '\n'
	// read: b'\n'
	// read: b'<hello xmlns="urn:ietf:params:xml:ns:netconf:base:1.0">\n <capabilities>\n  <capability>urn:ietf:params:netconf:base:1.1</capability>\n  <capability>urn:ietf:params:netconf:capability:candidate:1.0</capability>\n  <capability>urn:ietf:params:netconf:capability:rollback-on-error:1.0</capability>\n  <capability>urn:ietf:params:netconf:capability:validate:1.1</capability>\n  <capability>urn:ietf:params:netconf:capability:confirmed-commit:1.1</capability>\n  <capability>urn:ietf:params:netconf:capability:notification:1.0</capability>\n  <capability>urn:ietf:params:netconf:capability:interleave:1.0</capability>\n  <capability>http://cisco.com/ns/yang/Cisco-IOS-XR-infra-systemmib-cfg?module=Cisco-IOS-XR-infra-systemmib-cfg&amp;revision=2015-11-09</capability>\n  <capability>http://cisco.com/ns/yang/Cisco-IOS-XR-ipv4-autorp-datatypes?module=Cisco-IOS-XR-ipv4-autorp-datatypes&amp;revision=2015-11-09</capability>\n  <capability>http://cisco.com/ns/yang/Cisco-IOS-XR-perf-meas-cfg?module=Cisco-IOS-XR-perf-mea'
	// ound start of server capabilities, authentication successful
	ds := &DummyServer{
		block: make(chan struct{}),
	}
	handler := func  (s ssh.Session) {
		t.Logf("Handle request")
		time.Sleep(100 * time.Millisecond)
		// io.WriteString(s, "Banner\n\n")
		// send more that 8_192 without a linefeed
		io.WriteString(s, strings.Repeat("z", 8_192))
		io.WriteString(s, strings.Repeat("a", 10))
		// io.WriteString(s, `<hello xmlns="urn:ietf:params:xml:ns:netconf:base:1.0">\n <capabilities>\n  <capability>urn:ietf:params:netconf:base:1.1</capability>\n  <capability>urn:ietf:params:netconf:capability:candidate:1.0</capability>\n  <capability>urn:ietf:params:netconf:capability:rollback-on-error:1.0</capability>\n  <capability>urn:ietf:params:netconf:capability:validate:1.1</capability>\n  <capability>urn:ietf:params:netconf:capability:confirmed-commit:1.1</capability>\n  <capability>urn:ietf:params:netconf:capability:notification:1.0</capability>\n  <capability>urn:ietf:params:netconf:capability:interleave:1.0</capability>\n  <capability>http://cisco.com/ns/yang/Cisco-IOS-XR-infra-systemmib-cfg?module=Cisco-IOS-XR-infra-systemmib-cfg&amp;revision=2015-11-09</capability>\n  <capability>http://cisco.com/ns/yang/Cisco-IOS-XR-ipv4-autorp-datatypes?module=Cisco-IOS-XR-ipv4-autorp-datatypes&amp;revision=2015-11-09</capability>\n  <capability>http://cisco.com/ns/yang/Cisco-IOS-XR-perf-meas-cfg?module=Cisco-IOS-XR-perf-mea`)
		close(ds.block)
		t.Logf("send finished")
		time.Sleep(60 * time.Second)
		t.Logf("Request finished")
	}

	ds.s = &ssh.Server{
    Addr:             ":2222",
		SubsystemHandlers: map[string]ssh.SubsystemHandler{
			"netconf": handler,
		},
	}
	go func() {
		t.Log(ds.s.ListenAndServe())
	}()
	return ds
}
package transport_test

import (
	"io"
	"testing"

	"github.com/scrapli/scrapligo/driver/options"
	"github.com/scrapli/scrapligo/logging"
	"github.com/scrapli/scrapligo/transport"
)


func TestSystemTransport(t *testing.T) {
	device := dummyDevice(t)
	defer device.s.Close()
	sshArgs, err := transport.NewSSHArgs(
		options.WithAuthNoStrictKey(),
		options.WithSSHKnownHostsFile("/dev/null"),
	)
	if err != nil {
		t.Fatal(err)
	}
	sshArgs.NetconfConnection = true
	tp, err := transport.NewSystemTransport(sshArgs)

	if err != nil {
		t.Fatal(err)
	}
	openArgs, err := transport.NewArgs(
		&logging.Instance{},
		"localhost",
		options.WithPort(2222),
		options.WithAuthUsername("whatever"),
	)
	if err != nil {
		t.Fatal(err)
	}

	err = tp.Open(openArgs)

	if err != nil {
		t.Fatal(err)
	}

	doneChan := make(chan struct{})
	go func() {
		defer t.Log("read finished")
		defer close(doneChan)
		for {
			t.Log("starting to read")
			// defaultReadSize = 8_192
			b, err := tp.Read(8_192)
			t.Logf("read %d bytes: %s", len(b), b)
			if err != nil {
				if err == io.EOF {
					return
				}
				t.Errorf("failed to read : %s", err)
				return
			}
		}
	}()
	<- device.block

	t.Log("closing transport")
	err = tp.Close()
	if err != nil {
		t.Fatal(err)
	}
	t.Log("waiting for read to be done")
	<- doneChan
}
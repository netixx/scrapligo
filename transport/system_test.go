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
	defer device.Close()
	sshArgs, err := transport.NewSSHArgs()
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
	readChan := make(chan struct{})
	go func() {
		defer t.Log("read finished")
		defer close(doneChan)
		close(readChan)
		for {
			t.Log("starting to read")
			// defaultReadSize = 8_192
			b, err := tp.Read(8_192)
			t.Logf("read %d bytes", len(b))
			if err != nil {
				if err == io.EOF {
					return
				}
				t.Errorf("failed to read : %s", err)
				return
			}
		}
	}()
	<- readChan

	t.Log("closing transport")
	err = tp.Close()
	if err != nil {
		t.Fatal(err)
	}
	t.Log("waiting for read to be done")
	<- doneChan
}
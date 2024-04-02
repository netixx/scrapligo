package channel_test

import (
	"runtime"
	"testing"

	"github.com/scrapli/scrapligo/util"
)

func testRead(testName string, testCase *util.PayloadTestCase) func(t *testing.T) {
	return func(t *testing.T) {
		t.Logf("%s: starting", testName)

		c, _ := prepareChannel(t, testName, testCase.PayloadFile)

		defer c.Close()
		_, err := c.GetPrompt()
		if err != nil {
			t.Errorf("%s: encountered error running Channel GetPrompt, error: %s", testName, err)
		}
	}
}

func TestRead(t *testing.T) {
	cases := map[string]*util.PayloadTestCase{
		"get-prompt-simple": {
			Description: "simple get prompt test",
			PayloadFile: "get-prompt-simple.txt",
		},
		"get-prompt-a": {
			Description: "simple get prompt test",
			PayloadFile: "get-prompt-simple.txt",
		},
		"get-prompt-b": {
			Description: "simple get prompt test",
			PayloadFile: "get-prompt-simple.txt",
		},
		"get-prompt-c": {
			Description: "simple get prompt test",
			PayloadFile: "get-prompt-simple.txt",
		},
	}

	for testName, testCase := range cases {
		runtime.GC()
		before := &runtime.MemStats{}
		runtime.ReadMemStats(before)
		f := testRead(testName, testCase)
		t.Run(testName, f)
		runtime.GC()
		after := &runtime.MemStats{}
		runtime.ReadMemStats(after)

		t.Logf("alloc: before=%dB, after=%dB", before.Alloc, after.Alloc)
	}
}

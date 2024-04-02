package channel_test

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"runtime/pprof"
	"testing"

	"github.com/scrapli/scrapligo/util"
)

func testRead(testName string, testCase *util.PayloadTestCase) func(t *testing.T) {
	return func(t *testing.T) {
		t.Logf("%s: starting", testName)

		c, _ := prepareChannel(t, testName, testCase.PayloadFile)

		defer c.Close()
		_, err := c.ReadUntilAnyPrompt([]*regexp.Regexp{c.PromptPattern})
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
		"large-input": {
			Description: "simple get prompt test",
			PayloadFile: "large-input.txt",
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
		heapBaseFile, _ := os.Create(fmt.Sprintf("%s-base.heap", testName))
		pprof.WriteHeapProfile(heapBaseFile)
		heapBaseFile.Close()
		before := &runtime.MemStats{}
		runtime.ReadMemStats(before)
		f := testRead(testName, testCase)
		t.Run(testName, f)
		runtime.GC()
		after := &runtime.MemStats{}
		runtime.ReadMemStats(after)
		heapFile, _ := os.Create(fmt.Sprintf("%s.heap", testName))
		pprof.WriteHeapProfile(heapFile)
		heapFile.Close()
		t.Logf("alloc: before=%dB, after=%dB", before.Alloc, after.Alloc)
	}
}

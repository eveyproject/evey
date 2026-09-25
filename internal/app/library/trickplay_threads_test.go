package library

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/evey-project/evey/internal/pkg/cpubudget"
)

// On the global share a generation must leave room for the other tasks: sized
// on the whole share it held every slot and starved the scan behind a
// season's backlog.
func TestDecoderThreads_OneOnTheGlobalShareOwnShareOtherwise(t *testing.T) {
	b := cpubudget.New(8)
	s := &TrickplayService{}
	assert.Equal(t, 1, s.decoderThreads(), "no budget")

	s.WithCPUBudget(b)
	b.SetPolicy(cpubudget.Policy{GlobalPercent: 50})
	assert.Equal(t, 1, s.decoderThreads(), "global share: one thread, whatever its size")

	b.SetPolicy(cpubudget.Policy{GlobalPercent: 25, TaskPercent: map[string]int{trickplayTask: 50}})
	assert.Equal(t, 4, s.decoderThreads(), "its own share: the admin sized it")
}

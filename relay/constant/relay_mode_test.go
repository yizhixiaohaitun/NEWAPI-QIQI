package constant

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPath2RelayModeAsyncTasks(t *testing.T) {
	assert.Equal(t, RelayModeVideoSubmit, Path2RelayMode("/async/tasks"))
	assert.Equal(t, RelayModeVideoFetchByID, Path2RelayMode("/async/tasks/task_public"))
}

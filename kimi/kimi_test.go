package kimi

import "testing"

func TestSendChatRequest(t *testing.T) {
	result, err := SendChatRequest("https://world.hey.com/dhh/imperfections-create-connections-bc87d630")
	if err != nil {
		t.Error(err)
		return
	}

	t.Logf("resp: %v\n", result)
}

package session

import (
	"testing"

	"ai-identity-manager/internal/controller"
)

func connID(id, host, proc string, chains ...string) controller.Connection {
	c := conn(host, proc, chains...)
	c.ID = id
	return c
}

func TestDropsBetween(t *testing.T) {
	prev := []controller.Connection{
		connID("1", "api.anthropic.com", "claude.exe", "IPRoyal-Static", "AI-Static"),
		connID("2", "claude.ai", "claude.exe", "IPRoyal-Static", "AI-Static"),
		connID("9", "baidu.com", "wechat.exe", "DIRECT"),
	}
	cur := []controller.Connection{
		connID("1", "api.anthropic.com", "claude.exe", "IPRoyal-Static", "AI-Static"),
	}
	drops := DropsBetween(prev, cur)
	if len(drops) != 1 || drops[0].Conn.ID != "2" {
		t.Fatalf("drops = %+v", drops)
	}
}

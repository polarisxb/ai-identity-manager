package session

import "ai-identity-manager/internal/controller"

func DropsBetween(prev, cur []controller.Connection) []ConnVerdict {
	curIDs := make(map[string]bool, len(cur))
	for _, c := range cur {
		curIDs[c.ID] = true
	}
	var gone []ConnVerdict
	prevSnap := Evaluate(prev, "AI-Static", "IPRoyal-Static")
	for _, v := range prevSnap.AI {
		if !curIDs[v.Conn.ID] {
			gone = append(gone, v)
		}
	}
	return gone
}

package identity

func VerifyLevel(v VerifyResult) Level {
	if !v.Checked {
		return Yellow
	}
	if v.Error != "" {
		return Red
	}
	if !verifyHasExpectation(v) {
		return Yellow
	}
	if verifyMismatch(v) != "" {
		return Red
	}
	return Green
}

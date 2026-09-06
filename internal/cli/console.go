package cli

func wantsHiddenConsole(args []string) bool {
	hasKillSwitch := false
	hasNoPersist := false
	for _, arg := range args {
		switch arg {
		case "killswitch":
			hasKillSwitch = true
		case "--no-persist":
			hasNoPersist = true
		}
	}
	return hasKillSwitch && hasNoPersist
}

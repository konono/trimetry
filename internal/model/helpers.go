package model

func CountTrialStatuses(trials []Trial) (completed, failed, timeout, cancelled int) {
	for _, t := range trials {
		switch t.ExecutionStatus {
		case ExecStatusCompleted:
			completed++
		case ExecStatusFailed:
			failed++
		case ExecStatusTimeout:
			timeout++
		case ExecStatusCancelled:
			cancelled++
		}
	}
	return
}

func HasFailedTrials(trials []Trial) bool {
	for _, t := range trials {
		if t.ExecutionStatus != ExecStatusCompleted {
			return true
		}
	}
	return false
}

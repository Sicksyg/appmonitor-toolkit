package main

import "testing"

func TestInstalledIOSAnalysisChoice(t *testing.T) {
	tests := []struct {
		choice           string
		analyzeInstalled bool
		cancelled        bool
	}{
		{choice: installLatestIOSAppChoice},
		{choice: analyzeInstalledIOSAppChoice, analyzeInstalled: true},
		{choice: cancelIOSAnalysisChoice, cancelled: true},
		{choice: "", cancelled: true},
	}

	for _, test := range tests {
		analyzeInstalled, cancelled := installedIOSAnalysisChoice(test.choice)
		if analyzeInstalled != test.analyzeInstalled || cancelled != test.cancelled {
			t.Errorf("installedIOSAnalysisChoice(%q) = (%v, %v), want (%v, %v)",
				test.choice, analyzeInstalled, cancelled, test.analyzeInstalled, test.cancelled)
		}
	}
}

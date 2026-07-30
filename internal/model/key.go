package model

import "fmt"

func ScenarioModelKey(scenarioID, modelName string) string {
	return fmt.Sprintf("%s/%s", scenarioID, modelName)
}

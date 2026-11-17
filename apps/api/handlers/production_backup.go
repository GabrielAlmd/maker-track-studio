package handlers

import (
	"encoding/json"
	"errors"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/google/uuid"
)

func validateProductionBackup(jobs []models.PrintJob, runs []models.PrintRun, items []models.PrintRunItem, materials []models.PrintJobFilament, operations []models.ProductionOperation) error {
	invalid := errors.New("Backup contains invalid production relationships or lifecycle data")
	jobMap := map[uuid.UUID]models.PrintJob{}
	for _, j := range jobs {
		if j.WorkflowVersion > 2 || j.Revision < 0 {
			return invalid
		}
		jobMap[j.ID] = j
	}
	runMap := map[uuid.UUID]models.PrintRun{}
	for _, r := range runs {
		if _, ok := jobMap[r.PrintJobID]; !ok {
			return invalid
		}
		runMap[r.ID] = r
	}
	for _, i := range items {
		r, ok := runMap[i.PrintRunID]
		if !ok {
			return invalid
		}
		r.Items = append(r.Items, i)
		runMap[r.ID] = r
	}
	runCounts := map[uuid.UUID]int{}
	accepted := map[uuid.UUID]int64{}
	for _, r := range runMap {
		runCounts[r.PrintJobID]++
		for _, item := range r.Items {
			if r.Status == "completed" && r.Provenance == "verified" {
				accepted[r.PrintJobID] += item.Accepted
			}
		}
		if models.ValidatePrintRun(r) != nil {
			return invalid
		}
	}
	for _, job := range jobs {
		if job.WorkflowVersion == 2 && (runCounts[job.ID] == 0 || int64(job.UnitsPrinted) != accepted[job.ID]) {
			return invalid
		}
	}
	for _, m := range materials {
		if m.PrintRunID != nil {
			r, ok := runMap[*m.PrintRunID]
			if !ok || r.PrintJobID != m.PrintJobID {
				return invalid
			}
		} else if jobMap[m.PrintJobID].WorkflowVersion == 2 {
			return invalid
		}
	}
	for _, op := range operations {
		if _, ok := jobMap[op.PrintJobID]; !ok {
			return invalid
		}
		var response ProductionResponse
		if json.Unmarshal([]byte(op.Response), &response) != nil || response.ID != op.PrintJobID || response.OperationID != op.ID || len(op.PayloadHash) != 64 {
			return invalid
		}
	}
	return nil
}

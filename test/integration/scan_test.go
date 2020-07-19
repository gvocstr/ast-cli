// +build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"strconv"
	"testing"
	"time"

	"github.com/spf13/viper"

	scansRESTApi "github.com/checkmarxDev/scans/pkg/api/scans/rest/v1"
	"gotest.tools/assert/cmp"

	"gotest.tools/assert"
)

const (
	scanResultsNum    = 379
	incScanResultsNum = 22
)

func TestScansE2E(t *testing.T) {
	scanID, projectID := createScanSourcesFile(t)
	defer deleteProject(t, projectID)

	fullScanWaitTime := viper.GetInt("TEST_FULL_SCAN_WAIT_COMPLETED_SECONDS")
	incScanWaitTime := viper.GetInt("TEST_INC_SCAN_WAIT_COMPLETED_SECONDS")
	scanCompleted := pollScanUntilStatus(t, scanID, scansRESTApi.ScanCompleted, fullScanWaitTime, 5)
	assert.Assert(t, scanCompleted, "Full scan should be completed")

	scanResults := getResultsNumberForScan(t, scanID)
	log.Println("Full scan results number is", scanResults)
	assert.Equal(t, scanResults, scanResultsNum, "Wrong number of scan results")
	deleteScan(t, scanID)

	incScanID, _ := createIncScan(t)
	incScanCompleted := pollScanUntilStatus(t, incScanID, scansRESTApi.ScanCompleted, incScanWaitTime, 5)
	assert.Assert(t, incScanCompleted, "Incremental scan should be completed")

	incScanResults := getResultsNumberForScan(t, incScanID)
	log.Println("Incremental scan results number is", incScanResults)
	assert.Equal(t, incScanResults, incScanResultsNum, "Wrong number of inc scan results")

	listScans(t)
	getScansTags(t)
	deleteScan(t, incScanID)
}

func createScanSourcesFile(t *testing.T) (string, string) {
	// Create a full scan
	b := bytes.NewBufferString("")
	createCommand := createASTIntegrationTestCommand(t)
	createCommand.SetOut(b)
	err := execute(createCommand, "-v", "--format", "json", "scan", "create", "--input-file", "scan_payload.json", "--sources", "sources.zip")
	assert.NilError(t, err, "Creating a scan should pass")
	// Read response from buffer
	var createdScanJSON []byte
	createdScanJSON, err = ioutil.ReadAll(b)
	assert.NilError(t, err, "Reading scan response JSON should pass")
	createdScan := scansRESTApi.ScanResponseModel{}
	err = json.Unmarshal(createdScanJSON, &createdScan)
	assert.NilError(t, err, "Parsing scan response JSON should pass")
	assert.Assert(t, createdScan.Status == scansRESTApi.ScanCreated)
	log.Printf("Scan ID %s created in test", createdScan.ID)
	return createdScan.ID, createdScan.ProjectID
}

func deleteScan(t *testing.T, scanID string) {
	deleteScanCommand := createASTIntegrationTestCommand(t)
	err := execute(deleteScanCommand, "scan", "delete", scanID)
	assert.NilError(t, err, "Deleting a scan should pass")
}

func listScans(t *testing.T) {
	b := bytes.NewBufferString("")
	getAllCommand := createASTIntegrationTestCommand(t)
	getAllCommand.SetOut(b)
	var limit uint64 = 40
	var offset uint64 = 0

	lim := fmt.Sprintf("limit=%s", strconv.FormatUint(limit, 10))
	off := fmt.Sprintf("offset=%s", strconv.FormatUint(offset, 10))

	err := execute(getAllCommand, "-v", "--format", "json", "scan", "list", "--filter", lim, "--filter", off)
	assert.NilError(t, err, "Getting all scans should pass")
	// Read response from buffer
	var getAllJSON []byte
	getAllJSON, err = ioutil.ReadAll(b)
	assert.NilError(t, err, "Reading all scans response JSON should pass")
	allScans := []scansRESTApi.ScanResponseModel{}
	err = json.Unmarshal(getAllJSON, &allScans)
	assert.NilError(t, err, "Parsing all scans response JSON should pass")
}

func listScansList(t *testing.T) {
	getAllCommand := createASTIntegrationTestCommand(t)
	var limit uint64 = 40
	var offset uint64 = 0
	lim := fmt.Sprintf("limit=%s", strconv.FormatUint(limit, 10))
	off := fmt.Sprintf("offset=%s", strconv.FormatUint(offset, 10))
	err := execute(getAllCommand, "-v", "--format", "list", "scan", "list", "--filter", lim, "--filter", off)
	assert.NilError(t, err, "Getting all scans should pass")
}

func getScanByID(t *testing.T, scanID string) *scansRESTApi.ScanResponseModel {
	getBuffer := bytes.NewBufferString("")
	getCommand := createASTIntegrationTestCommand(t)
	getCommand.SetOut(getBuffer)
	err := execute(getCommand, "-v", "--format", "json", "scan", "show", scanID)
	assert.NilError(t, err)
	// Read response from buffer
	var getScanJSON []byte
	getScanJSON, err = ioutil.ReadAll(getBuffer)
	assert.NilError(t, err, "Reading scan response JSON should pass")
	getScan := scansRESTApi.ScanResponseModel{}
	err = json.Unmarshal(getScanJSON, &getScan)
	assert.NilError(t, err, "Parsing scan response JSON should pass")
	assert.Assert(t, cmp.Equal(getScan.ID, scanID))
	return &getScan
}
func getScanByIDList(t *testing.T, scanID string) {
	getCommand := createASTIntegrationTestCommand(t)
	err := execute(getCommand, "-v", "--format", "list", "scan", "show", scanID)
	assert.NilError(t, err)
}

func getScansTags(t *testing.T) {
	b := bytes.NewBufferString("")
	tagsCommand := createASTIntegrationTestCommand(t)
	tagsCommand.SetOut(b)
	err := execute(tagsCommand, "-v", "--format", "json", "scan", "tags")
	assert.NilError(t, err, "Getting tags should pass")
	// Read response from buffer
	var tagsJSON []byte
	tagsJSON, err = ioutil.ReadAll(b)
	assert.NilError(t, err, "Reading tags JSON should pass")
	tags := map[string][]string{}
	err = json.Unmarshal(tagsJSON, &tags)
	assert.NilError(t, err, "Parsing tags JSON should pass")
}

func createIncScan(t *testing.T) (string, string) {
	// Create an incremental scan
	incBuff := bytes.NewBufferString("")
	createIncCommand := createASTIntegrationTestCommand(t)
	createIncCommand.SetOut(incBuff)
	err := execute(createIncCommand, "-v", "--format", "json", "scan", "create", "--input-file", "scan_inc_payload.json", "--sources", "sources_inc.zip")
	assert.NilError(t, err, "Creating an incremental scan should pass")
	// Read response from buffer
	var createdIncScanJSON []byte
	createdIncScanJSON, err = ioutil.ReadAll(incBuff)
	assert.NilError(t, err, "Reading incremental scan response JSON should pass")
	createdIncScan := scansRESTApi.ScanResponseModel{}
	err = json.Unmarshal(createdIncScanJSON, &createdIncScan)
	assert.NilError(t, err, "Parsing incremental scan response JSON should pass")
	assert.Assert(t, createdIncScan.Status == scansRESTApi.ScanCreated)
	return createdIncScan.ID, createdIncScan.ProjectID
}

func pollScanUntilStatus(t *testing.T, scanID string, requiredStatus scansRESTApi.ScanStatus, timeout, sleep int) bool {
	log.Printf("Set timeout of %d seconds for the scan to complete...\n", timeout)
	// Wait for the scan to finish. See it's completed successfully
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return false
		default:
			log.Printf("Polling scan %s\n", scanID)
			scan := getScanByID(t, scanID)
			getScanByIDList(t, scanID)
			if s := string(scan.Status); s == string(requiredStatus) {
				return true
			} else if s == scansRESTApi.ScanFailed || s == scansRESTApi.ScanCanceled ||
				s == scansRESTApi.ScanCompleted {
				return false
			} else {
				time.Sleep(time.Duration(sleep) * time.Second)
			}
		}
	}
}

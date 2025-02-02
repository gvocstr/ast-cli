package commands

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/checkmarx/ast-cli/internal/commands/util"
	"github.com/checkmarx/ast-cli/internal/commands/util/printer"

	commonParams "github.com/checkmarx/ast-cli/internal/params"

	"github.com/checkmarx/ast-cli/internal/wrappers"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

const (
	failedCreatingSummary    = "Failed creating summary"
	failedGettingScan        = "Failed getting scan"
	failedListingResults     = "Failed listing results"
	failedListingCodeBashing = "Failed codebashing link"
	mediumLabel              = "medium"
	highLabel                = "high"
	lowLabel                 = "low"
	infoLabel                = "info"
	sonarTypeLabel           = "_sonar"
	directoryPermission      = 0700
	infoSonar                = "INFO"
	lowSonar                 = "MINOR"
	mediumSonar              = "MAJOR"
	highSonar                = "CRITICAL"
	infoLowSarif             = "note"
	mediumSarif              = "warning"
	highSarif                = "error"
	vulnerabilitySonar       = "VULNERABILITY"
	infoCx                   = "INFO"
	lowCx                    = "LOW"
	mediumCx                 = "MEDIUM"
	highCx                   = "HIGH"
	codeBashingKey           = "cb-url"
	failedGettingBfl         = "Failed getting BFL"
	notAvailableString       = "N/A"
	notAvailableNumber       = -1
	defaultPaddingSize       = -14
	scanPendingMessage       = "Scan triggered in asynchronous mode or still running. Click more details to get the full status."
	scaType                  = "sca"
	directDependencyType     = "Direct Dependency"
	indirectDependencyType   = "Transitive Dependency"
	startedStatus            = "started"

	completedStatus           = "completed"
	pdfToEmailFlagDescription = "Send the PDF report to the specified email address." +
		" Use \",\" as the delimiter for multiple emails"
	pdfOptionsFlagDescription = "Sections to generate PDF report. Available options: Iac-Security,Sast,Sca," +
		defaultPdfOptionsDataSections
	delayValueForPdfReport                  = 150
	reportNameScanReport                    = "scan-report"
	reportTypeEmail                         = "email"
	defaultPdfOptionsDataSections           = "ScanSummary,ExecutiveSummary,ScanResults"
	exploitablePathFlagDescription          = "Enable or disable exploitable path in scan. Available options: true,false"
	scaLastScanTimeFlagDescription          = "SCA last scan time. Available options: integer above 1"
	projectPrivatePackageFlagDescription    = "Enable or disable project private package. Available options: true,false"
	scaPrivatePackageVersionFlagDescription = "SCA project private package version. Example: 0.1.1"
)

var filterResultsListFlagUsage = fmt.Sprintf(
	"Filter the list of results. Use ';' as the delimiter for arrays. Available filters are: %s",
	strings.Join(
		[]string{
			commonParams.ScanIDQueryParam,
			commonParams.LimitQueryParam,
			commonParams.OffsetQueryParam,
			commonParams.SortQueryParam,
			commonParams.IncludeNodesQueryParam,
			commonParams.NodeIDsQueryParam,
			commonParams.QueryQueryParam,
			commonParams.GroupQueryParam,
			commonParams.StatusQueryParam,
			commonParams.SeverityQueryParam,
			commonParams.StateQueryParam,
		}, ",",
	),
)

var securities = map[string]string{
	infoCx:   "3.5",
	lowCx:    "6.5",
	mediumCx: "8.5",
	highCx:   "9.5",
}

// Match cx severity with sonar severity
var sonarSeverities = map[string]string{
	infoCx:   infoSonar,
	lowCx:    lowSonar,
	mediumCx: mediumSonar,
	highCx:   highSonar,
}

func NewResultsCommand(
	resultsWrapper wrappers.ResultsWrapper,
	scanWrapper wrappers.ScansWrapper,
	resultsPdfReportsWrapper wrappers.ResultsPdfWrapper,
	codeBashingWrapper wrappers.CodeBashingWrapper,
	bflWrapper wrappers.BflWrapper,
	risksOverviewWrapper wrappers.RisksOverviewWrapper,
) *cobra.Command {
	resultCmd := &cobra.Command{
		Use:   "results",
		Short: "Retrieve results",
		Annotations: map[string]string{
			"command:doc": heredoc.Doc(
				`
				https://checkmarx.com/resource/documents/en/34965-68640-results.html
			`,
			),
		},
	}
	showResultCmd := resultShowSubCommand(resultsWrapper, scanWrapper, resultsPdfReportsWrapper, risksOverviewWrapper)
	codeBashingCmd := resultCodeBashing(codeBashingWrapper)
	bflResultCmd := resultBflSubCommand(bflWrapper)
	resultCmd.AddCommand(
		showResultCmd, bflResultCmd, codeBashingCmd,
	)
	return resultCmd
}

func resultShowSubCommand(
	resultsWrapper wrappers.ResultsWrapper,
	scanWrapper wrappers.ScansWrapper,
	resultsPdfReportsWrapper wrappers.ResultsPdfWrapper,
	risksOverviewWrapper wrappers.RisksOverviewWrapper,
) *cobra.Command {
	resultShowCmd := &cobra.Command{
		Use:   "show",
		Short: "Show results of a scan",
		Long:  "The show command enables the ability to show results about a requested scan in Checkmarx One.",
		Example: heredoc.Doc(
			`
			$ cx results show --scan-id <scan Id>
		`,
		),
		RunE: runGetResultCommand(resultsWrapper, scanWrapper, resultsPdfReportsWrapper, risksOverviewWrapper),
	}
	addScanIDFlag(resultShowCmd, "ID to report on.")
	addResultFormatFlag(
		resultShowCmd,
		printer.FormatJSON,
		printer.FormatSummary,
		printer.FormatSummaryConsole,
		printer.FormatSarif,
		printer.FormatSummaryJSON,
		printer.FormatPDF,
		printer.FormatSummaryMarkdown,
	)
	resultShowCmd.PersistentFlags().String(commonParams.ReportFormatPdfToEmailFlag, "", pdfToEmailFlagDescription)
	resultShowCmd.PersistentFlags().String(commonParams.ReportFormatPdfOptionsFlag, defaultPdfOptionsDataSections, pdfOptionsFlagDescription)
	resultShowCmd.PersistentFlags().String(commonParams.TargetFlag, "cx_result", "Output file")
	resultShowCmd.PersistentFlags().String(commonParams.TargetPathFlag, ".", "Output Path")
	resultShowCmd.PersistentFlags().StringSlice(commonParams.FilterFlag, []string{}, filterResultsListFlagUsage)
	return resultShowCmd
}

func resultBflSubCommand(bflWrapper wrappers.BflWrapper) *cobra.Command {
	resultBflCmd := &cobra.Command{
		Use:   "bfl",
		Short: "Show best fix location for a query id within the scan result.",
		Long:  "The bfl command enables the ability to show best fix location for a querid within the scan result.",
		Example: heredoc.Doc(
			`
			$ cx results bfl --scan-id <scan Id> --query-id <query Id>
		`,
		),
		RunE: runGetBestFixLocationCommand(bflWrapper),
	}
	addScanIDFlag(resultBflCmd, "ID to report on.")
	addQueryIDFlag(resultBflCmd, "Query Id from the result.")
	addFormatFlag(resultBflCmd, printer.FormatList, printer.FormatJSON)

	markFlagAsRequired(resultBflCmd, commonParams.ScanIDFlag)
	markFlagAsRequired(resultBflCmd, commonParams.QueryIDFlag)

	return resultBflCmd
}

func runGetBestFixLocationCommand(bflWrapper wrappers.BflWrapper) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		var bflResponseModel *wrappers.BFLResponseModel
		var errorModel *wrappers.WebError
		var err error

		scanID, _ := cmd.Flags().GetString(commonParams.ScanIDFlag)
		queryID, _ := cmd.Flags().GetString(commonParams.QueryIDFlag)

		scanIds := strings.Split(scanID, ",")
		if len(scanIds) > 1 {
			return errors.Errorf("%s", "Multiple scan-ids are not allowed.")
		}
		queryIds := strings.Split(queryID, ",")
		if len(queryIds) > 1 {
			return errors.Errorf("%s", "Multiple query-ids are not allowed.")
		}

		params := make(map[string]string)
		params[commonParams.ScanIDQueryParam] = scanID
		params[commonParams.QueryIDQueryParam] = queryID

		bflResponseModel, errorModel, err = bflWrapper.GetBflByScanIDAndQueryID(params)

		if err != nil {
			return errors.Wrapf(err, "%s", failedGettingBfl)
		}

		// Checking the response
		if errorModel != nil {
			return errors.Errorf("%s: CODE: %d, %s", failedGettingBfl, errorModel.Code, errorModel.Message)
		} else if bflResponseModel != nil {
			err = printByFormat(cmd, toBflView(*bflResponseModel))
			if err != nil {
				return err
			}
		}

		return nil
	}
}

func toBflView(bflResponseModel wrappers.BFLResponseModel) []wrappers.ScanResultNode {
	if (bflResponseModel.TotalCount) > 0 {
		views := make([]wrappers.ScanResultNode, bflResponseModel.TotalCount)

		for i := 0; i < bflResponseModel.TotalCount; i++ {
			views[i] = wrappers.ScanResultNode{
				Name:       bflResponseModel.Trees[i].BFL.Name,
				FileName:   bflResponseModel.Trees[i].BFL.FileName,
				FullName:   bflResponseModel.Trees[i].BFL.FullName,
				Column:     bflResponseModel.Trees[i].BFL.Column,
				Length:     bflResponseModel.Trees[i].BFL.Length,
				Line:       bflResponseModel.Trees[i].BFL.Line,
				MethodLine: bflResponseModel.Trees[i].BFL.MethodLine,
				Method:     bflResponseModel.Trees[i].BFL.Method,
				DomType:    bflResponseModel.Trees[i].BFL.DomType,
			}
		}
		return views
	}
	views := make([]wrappers.ScanResultNode, 0)
	return views
}

func resultCodeBashing(codeBashingWrapper wrappers.CodeBashingWrapper) *cobra.Command {
	// Create a codeBashing wrapper
	resultCmd := &cobra.Command{
		Use:   "codebashing",
		Short: "Get codebashing lesson link",
		Long:  "The codebashing command enables the ability to retrieve the link about a specific vulnerability.",
		Example: heredoc.Doc(
			`
			$ cx results codebashing --language <string> --vulnerability-type <string> --cwe-id <string> --format <string>
		`,
		),
		RunE: runGetCodeBashingCommand(codeBashingWrapper),
	}
	resultCmd.PersistentFlags().String(commonParams.LanguageFlag, "", "Language of the vulnerability")
	err := resultCmd.MarkPersistentFlagRequired(commonParams.LanguageFlag)
	if err != nil {
		log.Fatal(err)
	}
	resultCmd.PersistentFlags().String(commonParams.VulnerabilityTypeFlag, "", "Vulnerability type")
	err = resultCmd.MarkPersistentFlagRequired(commonParams.VulnerabilityTypeFlag)
	if err != nil {
		log.Fatal(err)
	}
	resultCmd.PersistentFlags().String(commonParams.CweIDFlag, "", "CWE ID for the vulnerability")
	err = resultCmd.MarkPersistentFlagRequired(commonParams.CweIDFlag)
	if err != nil {
		log.Fatal(err)
	}
	addFormatFlag(resultCmd, printer.FormatJSON, printer.FormatTable, printer.FormatList)
	return resultCmd
}

func convertScanToResultsSummary(scanInfo *wrappers.ScanResponseModel) (*wrappers.ResultSummary, error) {
	if scanInfo == nil {
		return nil, errors.New(failedCreatingSummary)
	}

	sastIssues := 0
	scaIssues := 0
	kicsIssues := 0
	if len(scanInfo.StatusDetails) > 0 {
		for _, statusDetailItem := range scanInfo.StatusDetails {
			if statusDetailItem.Status == wrappers.ScanFailed || statusDetailItem.Status == wrappers.ScanCanceled {
				if statusDetailItem.Name == commonParams.SastType {
					sastIssues = notAvailableNumber
				} else if statusDetailItem.Name == commonParams.ScaType {
					scaIssues = notAvailableNumber
				} else if statusDetailItem.Name == commonParams.KicsType {
					kicsIssues = notAvailableNumber
				}
			}
		}
	}

	return &wrappers.ResultSummary{
		ScanID:         scanInfo.ID,
		Status:         string(scanInfo.Status),
		CreatedAt:      scanInfo.CreatedAt.Format("2006-01-02, 15:04:05"),
		ProjectID:      scanInfo.ProjectID,
		RiskStyle:      "",
		RiskMsg:        "",
		HighIssues:     0,
		MediumIssues:   0,
		LowIssues:      0,
		InfoIssues:     0,
		SastIssues:     sastIssues,
		KicsIssues:     kicsIssues,
		ScaIssues:      scaIssues,
		Tags:           scanInfo.Tags,
		ProjectName:    scanInfo.ProjectName,
		BranchName:     scanInfo.Branch,
		EnginesEnabled: scanInfo.Engines,
	}, nil
}

func SummaryReport(
	results *wrappers.ScanResultsCollection,
	scan *wrappers.ScanResponseModel,
	risksOverviewWrapper wrappers.RisksOverviewWrapper,
	resultsWrapper wrappers.ResultsWrapper,
) (*wrappers.ResultSummary, error) {
	summary, err := convertScanToResultsSummary(scan)
	if err != nil {
		return nil, err
	}

	baseURI, err := resultsWrapper.GetResultsURL(summary.ProjectID)
	if err != nil {
		return nil, err
	}

	summary.BaseURI = baseURI
	summary.BaseURI = generateScanSummaryURL(summary)
	if summary.HasAPISecurity() {
		apiSecRisks, err := getResultsForAPISecScanner(risksOverviewWrapper, summary.ScanID)
		if err != nil {
			return nil, err
		}

		summary.APISecurity = *apiSecRisks
	}

	for _, result := range results.Results {
		countResult(summary, result)
	}
	if summary.SastIssues == 0 {
		summary.SastIssues = notAvailableNumber
	}
	if summary.ScaIssues == 0 {
		summary.ScaIssues = notAvailableNumber
	}
	if summary.KicsIssues == 0 {
		summary.KicsIssues = notAvailableNumber
	}
	if summary.HighIssues > 0 {
		summary.RiskStyle = highLabel
		summary.RiskMsg = "High Risk"
	} else if summary.MediumIssues > 0 {
		summary.RiskStyle = mediumLabel
		summary.RiskMsg = "Medium Risk"
	} else if summary.LowIssues > 0 {
		summary.RiskStyle = lowLabel
		summary.RiskMsg = "Low Risk"
	} else if summary.TotalIssues == 0 {
		summary.RiskMsg = "No Risk"
	}
	return summary, nil
}

func countResult(summary *wrappers.ResultSummary, result *wrappers.ScanResult) {
	engineType := strings.TrimSpace(result.Type)
	if contains(summary.EnginesEnabled, engineType) && isExploitable(result.State) {
		if engineType == commonParams.SastType {
			summary.SastIssues++
			summary.TotalIssues++
		} else if engineType == commonParams.ScaType {
			summary.ScaIssues++
			summary.TotalIssues++
		} else if engineType == commonParams.KicsType {
			summary.KicsIssues++
			summary.TotalIssues++
		}
		severity := strings.ToLower(result.Severity)
		if severity == highLabel {
			summary.HighIssues++
		} else if severity == lowLabel {
			summary.LowIssues++
		} else if severity == mediumLabel {
			summary.MediumIssues++
		} else if severity == infoLabel {
			summary.InfoIssues++
		}
	}
}

func writeHTMLSummary(targetFile string, summary *wrappers.ResultSummary) error {
	log.Println("Creating Summary Report: ", targetFile)
	summaryTemp, err := template.New("summaryTemplate").Parse(wrappers.SummaryTemplate(isScanPending(summary.Status)))
	if err == nil {
		f, err := os.Create(targetFile)
		if err == nil {
			_ = summaryTemp.ExecuteTemplate(f, "SummaryTemplate", summary)
			_ = f.Close()
		}
		return err
	}
	return nil
}
func writeMarkdownSummary(targetFile string, data *wrappers.ResultSummary) error {
	log.Println("Creating Markdown Summary Report: ", targetFile)
	tmpl, err := template.New(printer.FormatSummaryMarkdown).Parse(wrappers.SummaryMarkdownTemplate)
	if err != nil {
		return err
	}
	file, err := os.Create(targetFile)
	if err != nil {
		return err
	}
	defer file.Close()

	err = tmpl.Execute(file, &data)
	if err != nil {
		return err
	}
	return nil
}

func writeConsoleSummary(summary *wrappers.ResultSummary) error {
	if !isScanPending(summary.Status) {
		fmt.Printf("            Scan Summary:                     \n")
		fmt.Printf("              Created At: %s\n", summary.CreatedAt)
		fmt.Printf("              Project Name: %s                        \n", summary.ProjectName)
		fmt.Printf("              Scan ID: %s                             \n\n", summary.ScanID)
		fmt.Printf("            Results Summary:                     \n")
		fmt.Printf(
			"              Risk Level: %s																									 \n",
			summary.RiskMsg,
		)
		fmt.Printf("              -----------------------------------     \n")
		if summary.HasAPISecurity() {
			fmt.Printf(
				"              API Security - Total Detected APIs: %d                       \n",
				summary.APISecurity.APICount)
		}

		fmt.Printf("              Total Results: %d                       \n", summary.TotalIssues)
		fmt.Printf("              -----------------------------------     \n")
		fmt.Printf("              |             High: %*d|     \n", defaultPaddingSize, summary.HighIssues)
		fmt.Printf("              |           Medium: %*d|     \n", defaultPaddingSize, summary.MediumIssues)
		fmt.Printf("              |              Low: %*d|     \n", defaultPaddingSize, summary.LowIssues)
		fmt.Printf("              |             Info: %*d|     \n", defaultPaddingSize, summary.InfoIssues)
		fmt.Printf("              -----------------------------------     \n")

		if summary.KicsIssues == notAvailableNumber {
			fmt.Printf("              |     IAC-SECURITY: %*s|     \n", defaultPaddingSize, notAvailableString)
		} else {
			fmt.Printf("              |     IAC-SECURITY: %*d|     \n", defaultPaddingSize, summary.KicsIssues)
		}
		if summary.SastIssues == notAvailableNumber {
			fmt.Printf("              |             SAST: %*s|     \n", defaultPaddingSize, notAvailableString)
		} else {
			fmt.Printf("              |             SAST: %*d|     \n", defaultPaddingSize, summary.SastIssues)
			if summary.HasAPISecurity() {
				fmt.Printf(
					"              |               APIS WITH RISK: %d |     \n",
					summary.APISecurity.TotalRisksCount)
			}
		}
		if summary.ScaIssues == notAvailableNumber {
			fmt.Printf("              |              SCA: %*s|     \n", defaultPaddingSize, notAvailableString)
		} else {
			fmt.Printf("              |              SCA: %*d|     \n", defaultPaddingSize, summary.ScaIssues)
		}
		fmt.Printf("              -----------------------------------     \n")
		fmt.Printf("              Checkmarx One - Scan Summary & Details: %s\n", summary.BaseURI)
	} else {
		fmt.Printf("Scan executed in asynchronous mode or still running. Hence, no results generated.\n")

		fmt.Printf("For more information: %s\n", summary.BaseURI)
	}
	return nil
}

func generateScanSummaryURL(summary *wrappers.ResultSummary) string {
	summaryURL := fmt.Sprintf(
		strings.Replace(summary.BaseURI, "overview", "scans?id=%s&branch=%s", 1),
		summary.ScanID, url.QueryEscape(summary.BranchName),
	)
	return summaryURL
}

func runGetResultCommand(
	resultsWrapper wrappers.ResultsWrapper,
	scanWrapper wrappers.ScansWrapper,
	resultsPdfReportsWrapper wrappers.ResultsPdfWrapper,
	risksOverviewWrapper wrappers.RisksOverviewWrapper,
) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		targetFile, _ := cmd.Flags().GetString(commonParams.TargetFlag)
		targetPath, _ := cmd.Flags().GetString(commonParams.TargetPathFlag)
		format, _ := cmd.Flags().GetString(commonParams.TargetFormatFlag)
		formatPdfToEmail, _ := cmd.Flags().GetString(commonParams.ReportFormatPdfToEmailFlag)
		formatPdfOptions, _ := cmd.Flags().GetString(commonParams.ReportFormatPdfOptionsFlag)

		scanID, _ := cmd.Flags().GetString(commonParams.ScanIDFlag)
		params, err := getFilters(cmd)
		if err != nil {
			return errors.Wrapf(err, "%s", failedListingResults)
		}
		return CreateScanReport(
			resultsWrapper,
			risksOverviewWrapper,
			scanWrapper,
			resultsPdfReportsWrapper,
			scanID,
			format,
			formatPdfToEmail,
			formatPdfOptions,
			targetFile,
			targetPath,
			params)
	}
}

func runGetCodeBashingCommand(
	codeBashingWrapper wrappers.CodeBashingWrapper,
) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		language, _ := cmd.Flags().GetString(commonParams.LanguageFlag)
		cwe, _ := cmd.Flags().GetString(commonParams.CweIDFlag)
		vulType, _ := cmd.Flags().GetString(commonParams.VulnerabilityTypeFlag)
		params, err := codeBashingWrapper.BuildCodeBashingParams(
			[]wrappers.CodeBashingParamsCollection{
				{
					CweID:       "CWE-" + cwe,
					Language:    language,
					CxQueryName: strings.ReplaceAll(vulType, " ", "_"),
				},
			},
		)
		if err != nil {
			return err
		}
		// Fetch the cached token or a new one to obtain the codebashing URL incoded in the jwt token
		codeBashingURL, err := codeBashingWrapper.GetCodeBashingURL(codeBashingKey)
		if err != nil {
			return err
		}
		// Make the request to the api to obtain the codebashing link and send the codebashing url to enrich the path
		CodeBashingModel, webError, err := codeBashingWrapper.GetCodeBashingLinks(params, codeBashingURL)
		if err != nil {
			return err
		}
		if webError != nil {
			return errors.New(webError.Message)
		}
		err = printByFormat(cmd, *CodeBashingModel)
		if err != nil {
			return errors.Wrapf(err, "%s", failedListingCodeBashing)
		}
		return nil
	}
}

func CreateScanReport(
	resultsWrapper wrappers.ResultsWrapper,
	risksOverviewWrapper wrappers.RisksOverviewWrapper,
	scanWrapper wrappers.ScansWrapper,
	resultsPdfReportsWrapper wrappers.ResultsPdfWrapper,
	scanID,
	reportTypes,
	formatPdfToEmail,
	formatPdfOptions,
	targetFile,
	targetPath string,
	params map[string]string,
) error {
	if scanID == "" {
		return errors.Errorf("%s: Please provide a scan ID", failedListingResults)
	}
	err := createDirectory(targetPath)
	if err != nil {
		return err
	}
	scan, errorModel, scanErr := scanWrapper.GetByID(scanID)
	if scanErr != nil {
		return errors.Wrapf(scanErr, "%s", failedGetting)
	}
	if errorModel != nil {
		return errors.Errorf("%s: CODE: %d, %s", failedGettingScan, errorModel.Code, errorModel.Message)
	}

	results, err := ReadResults(resultsWrapper, scan, params)
	if err != nil {
		return err
	}

	summary, err := SummaryReport(results, scan, risksOverviewWrapper, resultsWrapper)
	if err != nil {
		return err
	}

	reportList := strings.Split(reportTypes, ",")
	for _, reportType := range reportList {
		err = createReport(reportType, formatPdfToEmail, formatPdfOptions, targetFile, targetPath, results, summary, resultsPdfReportsWrapper)
		if err != nil {
			return err
		}
	}
	return nil
}

func validateEmails(emailString string) ([]string, error) {
	re := regexp.MustCompile(`^[a-zA-Z0-9_.+-]+@[a-zA-Z0-9-]+\.[a-zA-Z0-9-.]+$`)
	emails := strings.Split(emailString, ",")
	var validEmails []string
	for _, emailStr := range emails {
		email := strings.TrimSpace(emailStr)
		if re.MatchString(email) {
			validEmails = append(validEmails, email)
		} else {
			return nil, errors.Errorf("report not sent, invalid email address: %s", email)
		}
	}
	return validEmails, nil
}

func getResultsForAPISecScanner(
	risksOverviewWrapper wrappers.RisksOverviewWrapper,
	scanID string,
) (results *wrappers.APISecResult, err error) {
	var apiSecResultsModel *wrappers.APISecResult
	var errorModel *wrappers.WebError

	apiSecResultsModel, errorModel, err = risksOverviewWrapper.GetAllAPISecRisksByScanID(scanID)
	if err != nil {
		return nil, errors.Wrapf(err, "%s", failedListingResults)
	}
	if errorModel != nil {
		return nil, errors.Errorf("%s: CODE: %d, %s", failedListingResults, errorModel.Code, errorModel.Message)
	} else if apiSecResultsModel != nil {
		return apiSecResultsModel, nil
	}
	return nil, nil
}

func isScanPending(scanStatus string) bool {
	return !(strings.EqualFold(scanStatus, "Completed") || strings.EqualFold(
		scanStatus,
		"Partial",
	) || strings.EqualFold(scanStatus, "Failed"))
}

func createReport(
	format,
	formatPdfToEmail,
	formatPdfOptions,
	targetFile,
	targetPath string,
	results *wrappers.ScanResultsCollection,
	summary *wrappers.ResultSummary,
	resultsPdfReportsWrapper wrappers.ResultsPdfWrapper,

) error {
	if isScanPending(summary.Status) {
		summary.ScanInfoMessage = scanPendingMessage
	}

	if printer.IsFormat(format, printer.FormatSarif) {
		sarifRpt := createTargetName(targetFile, targetPath, "sarif")
		return exportSarifResults(sarifRpt, results)
	}
	if printer.IsFormat(format, printer.FormatSonar) {
		sonarRpt := createTargetName(fmt.Sprintf("%s%s", targetFile, sonarTypeLabel), targetPath, "json")
		return exportSonarResults(sonarRpt, results)
	}
	if printer.IsFormat(format, printer.FormatJSON) {
		jsonRpt := createTargetName(targetFile, targetPath, "json")
		return exportJSONResults(jsonRpt, results)
	}
	if printer.IsFormat(format, printer.FormatSummaryConsole) {
		return writeConsoleSummary(summary)
	}
	if printer.IsFormat(format, printer.FormatSummary) {
		summaryRpt := createTargetName(targetFile, targetPath, "html")
		convertNotAvailableNumberToZero(summary)
		return writeHTMLSummary(summaryRpt, summary)
	}
	if printer.IsFormat(format, printer.FormatSummaryJSON) {
		summaryRpt := createTargetName(targetFile, targetPath, "json")
		convertNotAvailableNumberToZero(summary)
		return exportJSONSummaryResults(summaryRpt, summary)
	}
	if printer.IsFormat(format, printer.FormatPDF) {
		summaryRpt := createTargetName(targetFile, targetPath, printer.FormatPDF)
		return exportPdfResults(resultsPdfReportsWrapper, summary, summaryRpt, formatPdfToEmail, formatPdfOptions)
	}
	if printer.IsFormat(format, printer.FormatSummaryMarkdown) {
		summaryRpt := createTargetName(targetFile, targetPath, "md")
		convertNotAvailableNumberToZero(summary)
		return writeMarkdownSummary(summaryRpt, summary)
	}
	err := fmt.Errorf("bad report format %s", format)
	return err
}

func createTargetName(targetFile, targetPath, targetType string) string {
	return filepath.Join(targetPath, targetFile+"."+targetType)
}

func createDirectory(targetPath string) error {
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		log.Printf("\nOutput path not found: %s\n", targetPath)
		log.Printf("Creating directory: %s\n", targetPath)
		err = os.Mkdir(targetPath, directoryPermission)
		if err != nil {
			return err
		}
	}
	return nil
}

func ReadResults(
	resultsWrapper wrappers.ResultsWrapper,
	scan *wrappers.ScanResponseModel,
	params map[string]string,
) (results *wrappers.ScanResultsCollection, err error) {
	var resultsModel *wrappers.ScanResultsCollection
	var errorModel *wrappers.WebError

	params[commonParams.ScanIDQueryParam] = scan.ID
	resultsModel, errorModel, err = resultsWrapper.GetAllResultsByScanID(params)

	if err != nil {
		return nil, errors.Wrapf(err, "%s", failedListingResults)
	}
	if errorModel != nil {
		return nil, errors.Errorf("%s: CODE: %d, %s", failedListingResults, errorModel.Code, errorModel.Message)
	}

	if resultsModel != nil {
		resultsModel, err = enrichScaResults(resultsWrapper, scan, params, resultsModel)
		if err != nil {
			return nil, err
		}

		resultsModel.ScanID = scan.ID
		return resultsModel, nil
	}
	return nil, nil
}

func enrichScaResults(
	resultsWrapper wrappers.ResultsWrapper,
	scan *wrappers.ScanResponseModel,
	params map[string]string,
	resultsModel *wrappers.ScanResultsCollection,
) (*wrappers.ScanResultsCollection, error) {
	if util.Contains(scan.Engines, scaType) {
		// Get additional information to enrich sca results
		scaPackageModel, errorModel, err := resultsWrapper.GetAllResultsPackageByScanID(params)
		if errorModel != nil {
			return nil, errors.Errorf("%s: CODE: %d, %s", failedListingResults, errorModel.Code, errorModel.Message)
		}
		if err != nil {
			return nil, errors.Wrapf(err, "%s", failedListingResults)
		}
		// Get additional information to add the type information to the sca results
		scaTypeModel, errorModel, err := resultsWrapper.GetAllResultsTypeByScanID(params)
		if errorModel != nil {
			return nil, errors.Errorf("%s: CODE: %d, %s", failedListingResults, errorModel.Code, errorModel.Message)
		}
		if err != nil {
			return nil, errors.Wrapf(err, "%s", failedListingResults)
		}
		// Enrich sca results
		if scaPackageModel != nil {
			resultsModel = addPackageInformation(resultsModel, scaPackageModel, scaTypeModel)
		}
	}
	return resultsModel, nil
}

func exportSarifResults(targetFile string, results *wrappers.ScanResultsCollection) error {
	var err error
	var resultsJSON []byte
	log.Println("Creating SARIF Report: ", targetFile)
	var sarifResults = convertCxResultsToSarif(results)
	resultsJSON, err = json.Marshal(sarifResults)
	if err != nil {
		return errors.Wrapf(err, "%s: failed to serialize results response ", failedGettingAll)
	}
	f, err := os.Create(targetFile)
	if err != nil {
		return errors.Wrapf(err, "%s: failed to create target file  ", failedGettingAll)
	}
	_, _ = fmt.Fprintln(f, string(resultsJSON))
	_ = f.Close()
	return nil
}

func exportSonarResults(targetFile string, results *wrappers.ScanResultsCollection) error {
	var err error
	var resultsJSON []byte
	log.Println("Creating SONAR Report: ", targetFile)
	var sonarResults = convertCxResultsToSonar(results)
	resultsJSON, err = json.Marshal(sonarResults)
	if err != nil {
		return errors.Wrapf(err, "%s: failed to serialize results response ", failedGettingAll)
	}
	f, err := os.Create(targetFile)
	if err != nil {
		return errors.Wrapf(err, "%s: failed to create target file  ", failedGettingAll)
	}
	_, _ = fmt.Fprintln(f, string(resultsJSON))
	_ = f.Close()
	return nil
}
func exportJSONResults(targetFile string, results *wrappers.ScanResultsCollection) error {
	var err error
	var resultsJSON []byte
	log.Println("Creating JSON Report: ", targetFile)
	resultsJSON, err = json.Marshal(results)
	if err != nil {
		return errors.Wrapf(err, "%s: failed to serialize results response ", failedGettingAll)
	}
	f, err := os.Create(targetFile)
	if err != nil {
		return errors.Wrapf(err, "%s: failed to create target file  ", failedGettingAll)
	}
	_, _ = fmt.Fprintln(f, string(resultsJSON))
	_ = f.Close()
	return nil
}

func exportJSONSummaryResults(targetFile string, results *wrappers.ResultSummary) error {
	var err error
	var resultsJSON []byte
	log.Println("Creating summary JSON Report: ", targetFile)
	resultsJSON, err = json.Marshal(results)
	if err != nil {
		return errors.Wrapf(err, "%s: failed to serialize results response ", failedGettingAll)
	}
	f, err := os.Create(targetFile)
	if err != nil {
		return errors.Wrapf(err, "%s: failed to create target file  ", failedGettingAll)
	}
	_, _ = fmt.Fprintln(f, string(resultsJSON))
	_ = f.Close()
	return nil
}

func exportPdfResults(pdfWrapper wrappers.ResultsPdfWrapper, summary *wrappers.ResultSummary, summaryRpt, formatPdfToEmail, pdfOptions string) error {
	pdfReportsPayload := &wrappers.PdfReportsPayload{}
	poolingResp := &wrappers.PdfPoolingResponse{}

	pdfOptionsSections, pdfOptionsEngines, err := validatePdfOptions(pdfOptions)
	if err != nil {
		return err
	}
	pdfReportsPayload.ReportName = reportNameScanReport
	pdfReportsPayload.ReportType = "cli"
	pdfReportsPayload.FileFormat = printer.FormatPDF
	pdfReportsPayload.Data.ScanID = summary.ScanID
	pdfReportsPayload.Data.ProjectID = summary.ProjectID
	pdfReportsPayload.Data.BranchName = summary.BranchName
	pdfReportsPayload.Data.Scanners = pdfOptionsEngines
	pdfReportsPayload.Data.Sections = pdfOptionsSections

	// will generate pdf report and send it to the email list
	// instead of saving it to the file system
	if len(formatPdfToEmail) > 0 {
		emailList, validateErr := validateEmails(formatPdfToEmail)
		if validateErr != nil {
			return validateErr
		}
		pdfReportsPayload.ReportType = reportTypeEmail
		pdfReportsPayload.Data.Email = emailList
	}
	pdfReportID, webErr, err := pdfWrapper.GeneratePdfReport(pdfReportsPayload)
	if webErr != nil {
		return errors.Errorf("Error generating PDF report - %s", webErr.Message)
	}
	if err != nil {
		return errors.Errorf("Error generating PDF report - %s", err.Error())
	}

	if pdfReportsPayload.ReportType == reportTypeEmail {
		log.Println("Sending PDF report to: ", pdfReportsPayload.Data.Email)
		return nil
	}

	log.Println("Generating PDF report")
	poolingResp.Status = startedStatus
	for poolingResp.Status == startedStatus {
		poolingResp, webErr, err = pdfWrapper.CheckPdfReportStatus(pdfReportID.ReportID)
		if err != nil || webErr != nil {
			return errors.Wrapf(err, "%v", webErr)
		}
		time.Sleep(delayValueForPdfReport * time.Millisecond)
	}
	if poolingResp.Status != completedStatus {
		return errors.Errorf("PDF generating failed - Current status: %s", poolingResp.Status)
	}
	err = pdfWrapper.DownloadPdfReport(pdfReportID.ReportID, summaryRpt)
	if err != nil {
		return errors.Wrapf(err, "%s", "Failed downloading PDF report")
	}
	return nil
}

func validatePdfOptions(pdfOptions string) (pdfOptionsSections, pdfOptionsEngines []string, err error) {
	var pdfOptionsSectionsMap = map[string]string{
		"scansummary":      "ScanSummary",
		"executivesummary": "ExecutiveSummary",
		"scanresults":      "ScanResults",
	}
	var pdfOptionsEnginesMap = map[string]string{
		commonParams.ScaType:  "SCA",
		commonParams.SastType: "SAST",
		commonParams.KicsType: "KICS",
		commonParams.IacType:  "KICS",
	}
	pdfOptions = strings.ToLower(strings.ReplaceAll(pdfOptions, " ", ""))
	options := strings.Split(strings.ReplaceAll(pdfOptions, "\n", ""), ",")
	for _, s := range options {
		if pdfOptionsEnginesMap[s] != "" {
			pdfOptionsEngines = append(pdfOptionsEngines, pdfOptionsEnginesMap[s])
		} else if pdfOptionsSectionsMap[s] != "" {
			pdfOptionsSections = append(pdfOptionsSections, pdfOptionsSectionsMap[s])
		} else {
			return nil, nil, errors.Errorf("report option \"%s\" unavailable", s)
		}
	}
	return pdfOptionsSections, pdfOptionsEngines, nil
}

func convertCxResultsToSarif(results *wrappers.ScanResultsCollection) *wrappers.SarifResultsCollection {
	var sarif = new(wrappers.SarifResultsCollection)
	sarif.Schema = "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json"
	sarif.Version = "2.1.0"
	sarif.Runs = []wrappers.SarifRun{}
	sarif.Runs = append(sarif.Runs, createSarifRun(results))
	return sarif
}

func convertCxResultsToSonar(results *wrappers.ScanResultsCollection) *wrappers.ScanResultsSonar {
	var sonar = new(wrappers.ScanResultsSonar)
	sonar.Results = parseResultsSonar(results)
	return sonar
}

func createSarifRun(results *wrappers.ScanResultsCollection) wrappers.SarifRun {
	var sarifRun wrappers.SarifRun
	sarifRun.Tool.Driver.Name = wrappers.SarifName
	sarifRun.Tool.Driver.Version = wrappers.SarifVersion
	sarifRun.Tool.Driver.InformationURI = wrappers.SarifInformationURI
	sarifRun.Tool.Driver.Rules, sarifRun.Results = parseResults(results)
	return sarifRun
}

func parseResults(results *wrappers.ScanResultsCollection) ([]wrappers.SarifDriverRule, []wrappers.SarifScanResult) {
	var sarifRules []wrappers.SarifDriverRule
	var sarifResults []wrappers.SarifScanResult
	if results != nil {
		ruleIds := map[interface{}]bool{}
		for _, result := range results.Results {
			if rule := findRule(ruleIds, result); rule != nil {
				sarifRules = append(sarifRules, *rule)
			}
			if sarifResult := findResult(result); sarifResult != nil {
				sarifResults = append(sarifResults, sarifResult...)
			}
		}
	}
	return sarifRules, sarifResults
}

func parseResultsSonar(results *wrappers.ScanResultsCollection) []wrappers.SonarIssues {
	var sonarIssues []wrappers.SonarIssues

	if results != nil {
		for _, result := range results.Results {
			var auxIssue = initSonarIssue(result)

			engineType := strings.TrimSpace(result.Type)

			if engineType == commonParams.SastType {
				auxIssue.PrimaryLocation = parseSonarPrimaryLocation(result)
				auxIssue.SecondaryLocations = parseSonarSecondaryLocations(result)
				sonarIssues = append(sonarIssues, auxIssue)
			} else if engineType == commonParams.KicsType {
				auxIssue.PrimaryLocation = parseLocationKics(result)
				sonarIssues = append(sonarIssues, auxIssue)
			} else if engineType == commonParams.ScaType {
				sonarIssuesByLocation := parseScaSonarLocations(result)
				sonarIssues = append(sonarIssues, sonarIssuesByLocation...)
			}
		}
	}
	return sonarIssues
}

func initSonarIssue(result *wrappers.ScanResult) wrappers.SonarIssues {
	var sonarIssue wrappers.SonarIssues
	sonarIssue.Severity = sonarSeverities[result.Severity]
	sonarIssue.Type = vulnerabilitySonar
	sonarIssue.EngineID = result.Type
	sonarIssue.RuleID = result.ID
	sonarIssue.EffortMinutes = 0

	return sonarIssue
}

func parseScaSonarLocations(result *wrappers.ScanResult) []wrappers.SonarIssues {
	if result == nil || result.ScanResultData.ScaPackageCollection == nil || result.ScanResultData.ScaPackageCollection.Locations == nil {
		return []wrappers.SonarIssues{}
	}

	var issuesByLocation []wrappers.SonarIssues

	for _, location := range result.ScanResultData.ScaPackageCollection.Locations {
		issueByLocation := initSonarIssue(result)

		var primaryLocation wrappers.SonarLocation

		primaryLocation.FilePath = *location
		_, _, primaryLocation.Message = findRuleID(result)

		var textRange wrappers.SonarTextRange
		textRange.StartColumn = 1
		textRange.EndColumn = 2
		textRange.StartLine = 1
		textRange.EndLine = 2

		primaryLocation.TextRange = textRange

		issueByLocation.PrimaryLocation = primaryLocation

		issuesByLocation = append(issuesByLocation, issueByLocation)
	}

	return issuesByLocation
}

func parseLocationKics(results *wrappers.ScanResult) wrappers.SonarLocation {
	var auxLocation wrappers.SonarLocation
	auxLocation.FilePath = strings.TrimLeft(results.ScanResultData.Filename, "/")
	auxLocation.Message = results.ScanResultData.Value
	var auxTextRange wrappers.SonarTextRange
	auxTextRange.StartLine = results.ScanResultData.Line
	auxTextRange.StartColumn = 0
	auxTextRange.EndColumn = 1
	auxLocation.TextRange = auxTextRange
	return auxLocation
}

func parseSonarPrimaryLocation(results *wrappers.ScanResult) wrappers.SonarLocation {
	var auxLocation wrappers.SonarLocation
	// fill the details in the primary Location
	if len(results.ScanResultData.Nodes) > 0 {
		auxLocation.FilePath = strings.TrimLeft(results.ScanResultData.Nodes[0].FileName, "/")
		auxLocation.Message = strings.ReplaceAll(results.ScanResultData.QueryName, "_", " ")
		auxLocation.TextRange = parseSonarTextRange(results.ScanResultData.Nodes[0])
	}
	return auxLocation
}

func parseSonarSecondaryLocations(results *wrappers.ScanResult) []wrappers.SonarLocation {
	var auxSecondaryLocations []wrappers.SonarLocation
	// Traverse all the rest of the scan result nodes into secondary location of sonar
	if len(results.ScanResultData.Nodes) >= 1 {
		for _, node := range results.ScanResultData.Nodes[1:] {
			var auxSecondaryLocation wrappers.SonarLocation
			auxSecondaryLocation.FilePath = strings.TrimLeft(node.FileName, "/")
			auxSecondaryLocation.Message = strings.ReplaceAll(results.ScanResultData.QueryName, "_", " ")
			auxSecondaryLocation.TextRange = parseSonarTextRange(node)
			auxSecondaryLocations = append(auxSecondaryLocations, auxSecondaryLocation)
		}
	}
	return auxSecondaryLocations
}

func parseSonarTextRange(results *wrappers.ScanResultNode) wrappers.SonarTextRange {
	var auxTextRange wrappers.SonarTextRange
	auxTextRange.StartLine = results.Line
	auxTextRange.StartColumn = results.Column
	auxTextRange.EndColumn = results.Column + results.Length
	if auxTextRange.StartColumn == auxTextRange.EndColumn {
		auxTextRange.EndColumn++
	}
	return auxTextRange
}

func findRule(ruleIds map[interface{}]bool, result *wrappers.ScanResult) *wrappers.SarifDriverRule {
	var sarifRule wrappers.SarifDriverRule
	sarifRule.ID, sarifRule.Name, _ = findRuleID(result)
	sarifRule.FullDescription = findFullDescription(result)
	sarifRule.Help = findHelp(result)
	sarifRule.HelpURI = wrappers.SarifInformationURI
	sarifRule.Properties = findProperties(result)

	if !ruleIds[sarifRule.ID] {
		ruleIds[sarifRule.ID] = true
		return &sarifRule
	}

	return nil
}

func findRuleID(result *wrappers.ScanResult) (ruleID, ruleName, shortMessage string) {
	if result.ScanResultData.QueryID == nil {
		return fmt.Sprintf("%s (%s)", result.ID, result.Type),
			strings.Title(strings.ToLower(strings.ReplaceAll(result.ID, "-", ""))),
			fmt.Sprintf("%s (%s)", result.ScanResultData.PackageIdentifier, result.ID)
	}

	return fmt.Sprintf("%v (%s)", result.ScanResultData.QueryID, result.Type),
		strings.ReplaceAll(result.ScanResultData.QueryName, "_", " "),
		strings.ReplaceAll(result.ScanResultData.QueryName, "_", " ")
}

func findFullDescription(result *wrappers.ScanResult) wrappers.SarifDescription {
	var sarifDescription wrappers.SarifDescription
	sarifDescription.Text = findDescriptionText(result)
	return sarifDescription
}

func findHelp(result *wrappers.ScanResult) wrappers.SarifHelp {
	var sarifHelp wrappers.SarifHelp
	sarifHelp.Text = findDescriptionText(result)
	sarifHelp.Markdown = findHelpMarkdownText(result)

	return sarifHelp
}

func findDescriptionText(result *wrappers.ScanResult) string {
	if result.Type == commonParams.KicsType {
		return fmt.Sprintf(
			"%s Value: %s Excepted value: %s",
			result.Description, result.ScanResultData.Value, result.ScanResultData.ExpectedValue,
		)
	}

	return result.Description
}

func findHelpMarkdownText(result *wrappers.ScanResult) string {
	if result.Type == commonParams.KicsType {
		return fmt.Sprintf(
			"%s <br><br><strong>Value:</strong> %s <br><strong>Excepted value:</strong> %s",
			result.Description, result.ScanResultData.Value, result.ScanResultData.ExpectedValue,
		)
	}

	return result.Description
}

func findProperties(result *wrappers.ScanResult) wrappers.SarifProperties {
	var sarifProperties wrappers.SarifProperties
	sarifProperties.ID, sarifProperties.Name, _ = findRuleID(result)
	sarifProperties.Description = findDescriptionText(result)
	sarifProperties.SecuritySeverity = securities[result.Severity]
	sarifProperties.Tags = []string{"security", "checkmarx", result.Type}

	return sarifProperties
}

func findSarifLevel(result *wrappers.ScanResult) string {
	level := map[string]string{
		infoCx:   infoLowSarif,
		lowCx:    infoLowSarif,
		mediumCx: mediumSarif,
		highCx:   highSarif,
	}
	return level[result.Severity]
}

func initSarifResult(result *wrappers.ScanResult) wrappers.SarifScanResult {
	var scanResult wrappers.SarifScanResult
	scanResult.RuleID, _, scanResult.Message.Text = findRuleID(result)
	scanResult.Level = findSarifLevel(result)
	scanResult.Locations = []wrappers.SarifLocation{}

	return scanResult
}

func findResult(result *wrappers.ScanResult) []wrappers.SarifScanResult {
	var scanResults []wrappers.SarifScanResult

	if len(result.ScanResultData.Nodes) > 0 {
		scanResults = parseSarifResultSast(result, scanResults)
	} else if result.Type == commonParams.KicsType {
		scanResults = parseSarifResultKics(result, scanResults)
	} else if result.Type == commonParams.ScaType {
		scanResults = parseSarifResultsSca(result, scanResults)
	}

	if len(scanResults) > 0 {
		return scanResults
	}
	return nil
}

func parseSarifResultsSca(result *wrappers.ScanResult, scanResults []wrappers.SarifScanResult) []wrappers.SarifScanResult {
	if result == nil || result.ScanResultData.ScaPackageCollection == nil || result.ScanResultData.ScaPackageCollection.Locations == nil {
		return scanResults
	}
	for _, location := range result.ScanResultData.ScaPackageCollection.Locations {
		var scanResult = initSarifResult(result)

		var scanLocation wrappers.SarifLocation
		scanLocation.PhysicalLocation.ArtifactLocation.URI = *location
		scanLocation.PhysicalLocation.Region = &wrappers.SarifRegion{}
		scanLocation.PhysicalLocation.Region.StartLine = 1
		scanLocation.PhysicalLocation.Region.StartColumn = 1
		scanLocation.PhysicalLocation.Region.EndColumn = 2
		scanResult.Locations = append(scanResult.Locations, scanLocation)

		scanResults = append(scanResults, scanResult)
	}
	return scanResults
}

func parseSarifResultKics(result *wrappers.ScanResult, scanResults []wrappers.SarifScanResult) []wrappers.SarifScanResult {
	var scanResult = initSarifResult(result)
	var scanLocation wrappers.SarifLocation

	scanLocation.PhysicalLocation.ArtifactLocation.URI = strings.Replace(
		result.ScanResultData.Filename,
		"/",
		"",
		1,
	)
	scanLocation.PhysicalLocation.Region = &wrappers.SarifRegion{}
	scanLocation.PhysicalLocation.Region.StartLine = result.ScanResultData.Line
	scanLocation.PhysicalLocation.Region.StartColumn = 1
	scanLocation.PhysicalLocation.Region.EndColumn = 2
	scanResult.Locations = append(scanResult.Locations, scanLocation)

	scanResults = append(scanResults, scanResult)
	return scanResults
}

func parseSarifResultSast(result *wrappers.ScanResult, scanResults []wrappers.SarifScanResult) []wrappers.SarifScanResult {
	if result == nil || result.ScanResultData.Nodes == nil {
		return scanResults
	}
	var scanResult = initSarifResult(result)

	for _, node := range result.ScanResultData.Nodes {
		var scanLocation wrappers.SarifLocation
		scanLocation.PhysicalLocation.ArtifactLocation.URI = node.FileName[1:]
		if node.Line <= 0 {
			continue
		}
		scanLocation.PhysicalLocation.Region = &wrappers.SarifRegion{}
		scanLocation.PhysicalLocation.Region.StartLine = node.Line
		column := node.Column
		length := node.Length
		scanLocation.PhysicalLocation.Region.StartColumn = column
		scanLocation.PhysicalLocation.Region.EndColumn = column + length

		scanResult.Locations = append(scanResult.Locations, scanLocation)
	}

	scanResults = append(scanResults, scanResult)
	return scanResults
}

func convertNotAvailableNumberToZero(summary *wrappers.ResultSummary) {
	if summary.KicsIssues == notAvailableNumber {
		summary.KicsIssues = 0
	} else if summary.SastIssues == notAvailableNumber {
		summary.SastIssues = 0
	} else if summary.ScaIssues == notAvailableNumber {
		summary.ScaIssues = 0
	}
}

func buildAuxiliaryScaMaps(resultsModel *wrappers.ScanResultsCollection, scaPackageModel *[]wrappers.ScaPackageCollection,
	scaTypeModel *[]wrappers.ScaTypeCollection) (locationsByID map[string][]*string, typesByCVE map[string]string) {
	locationsByID = make(map[string][]*string)
	typesByCVE = make(map[string]string)
	// Create map to be used to populate locations for each package path
	for _, result := range resultsModel.Results {
		if result.Type == scaType {
			for _, packages := range *scaPackageModel {
				currentPackage := packages
				locationsByID[packages.ID] = currentPackage.Locations
			}
			for _, types := range *scaTypeModel {
				currentTypes := types
				typesByCVE[types.ID] = currentTypes.Type
			}
		}
	}
	return locationsByID, typesByCVE
}

func buildScaType(typesByCVE map[string]string, result *wrappers.ScanResult) string {
	types := typesByCVE[result.ID]
	if types == "SupplyChain" {
		return "Supply Chain"
	}
	return "Vulnerability"
}

func addPackageInformation(
	resultsModel *wrappers.ScanResultsCollection,
	scaPackageModel *[]wrappers.ScaPackageCollection,
	scaTypeModel *[]wrappers.ScaTypeCollection,
) *wrappers.ScanResultsCollection {
	var currentID string
	locationsByID, typesByCVE := buildAuxiliaryScaMaps(resultsModel, scaPackageModel, scaTypeModel)

	for _, result := range resultsModel.Results {
		if !(result.Type == scaType) {
			continue
		} else {
			currentID = result.ScanResultData.PackageIdentifier
			const precision = 1
			var roundedScore = util.RoundFloat(result.VulnerabilityDetails.CvssScore, precision)
			result.VulnerabilityDetails.CvssScore = roundedScore
			// Add the sca type
			result.ScaType = buildScaType(typesByCVE, result)
			for _, packages := range *scaPackageModel {
				currentPackage := packages
				if packages.ID == currentID {
					for _, dependencyPath := range currentPackage.DependencyPathArray {
						head := &dependencyPath[0]
						head.Locations = locationsByID[head.ID]
						head.SupportsQuickFix = len(dependencyPath) == 1
						for _, location := range locationsByID[head.ID] {
							head.SupportsQuickFix = head.SupportsQuickFix && util.IsPackageFileSupported(*location)
						}
						currentPackage.SupportsQuickFix = currentPackage.SupportsQuickFix || head.SupportsQuickFix
					}
					if result.VulnerabilityDetails.CveName != "" {
						currentPackage.FixLink = "https://devhub.checkmarx.com/cve-details/" + result.VulnerabilityDetails.CveName
					} else {
						currentPackage.FixLink = ""
					}
					if currentPackage.IsDirectDependency {
						currentPackage.TypeOfDependency = directDependencyType
					} else {
						currentPackage.TypeOfDependency = indirectDependencyType
					}
					result.ScanResultData.ScaPackageCollection = &currentPackage
					break
				}
			}
		}
	}
	return resultsModel
}

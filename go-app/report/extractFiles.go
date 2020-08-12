package report

import (
	"bufio"
	"compress/gzip"
	"errors"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"radar-log-parser/go-app/utilities"

	"github.com/PuerkitoBio/goquery"
	"gopkg.in/yaml.v2"
)

func extractConfig(cfgName string, bucket string, cfgFile *Config) error {
	cfg_data, err := utilities.DownloadFile(nil, bucket, cfgName)
	if err != nil {
		return err
	}
	cfg := &ConfigInterface{}
	if err := yaml.Unmarshal(cfg_data, cfg); err != nil {
		return err
	}
	cfgFile.IssuesGeneralFields.Details = cfg.IssuesGeneralFields.Details
	cfgFile.IssuesGeneralFields.Log_level = cfg.IssuesGeneralFields.Log_level
	cfgFile.IssuesGeneralFields.Number = cfg.IssuesGeneralFields.Number
	cfgFile.IssuesGeneralFields.OtherFields = cfg.IssuesGeneralFields.OtherFields
	cfgFile.IssuesGeneralFields.Timestamp = cfg.IssuesGeneralFields.Timestamp
	cfgFile.Priority = cfg.Priority
	cfgFile.SpecificProcess = cfg.SpecificProcess
	cfgFile.ImportantEvents = cfg.ImportantEvents
	cfgFile.Issues = make(map[string]Issue)
	for issue_name, _ := range cfg.Issues {
		cfgFile.Issues[issue_name] = extract_issues_content(cfg.Issues[issue_name])
	}
	return nil
}

func extract_issues_content(issue interface{}) Issue {
	myIssues := Issue{}
	myIssues.specific_process = make(map[string]string)
	myIssues.additional_fields = make(map[string]string)
	for issue_key, issue_value := range issue.(map[interface{}]interface{}) {
		switch issue_value.(type) {
		case string:
			switch issue_key {
			case "regex":
				myIssues.regex = issue_value.(string)
			case "detailing_mode":
				myIssues.detailing_mode = issue_value.(string)
			case "grouping":
				myIssues.grouping = issue_value.(string)
			}

		case map[interface{}]interface{}:
			for name, value := range issue_value.(map[interface{}]interface{}) {
				if issue_key == "specific_process" {
					myIssues.specific_process[name.(string)] = value.(string)
				} else {
					myIssues.additional_fields[name.(string)] = value.(string)
				}
			}
		case interface{}:
		}
	}
	return myIssues
}
func uploadLogFile(w http.ResponseWriter, r *http.Request, project_id string, region_id string) (string, *string, string, string, error) {
	r.ParseMultipartForm(10 << 20)
	cfg_file := r.FormValue("selectedFile")
	res, err := http.Get("https://" + project_id + "." + region_id + "." + "r.appspot.com/")
	if err != nil {
		return "", nil, cfg_file, "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", nil, cfg_file, "", err
	}
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return "", nil, cfg_file, "", err
	}
	selectedBucket, found := doc.Find("optgroup").Attr("label")
	if !found {
		return "", nil, cfg_file, "", err
	}
	file, handler, err := r.FormFile("myFile")
	if err != nil {
		return "", nil, cfg_file, selectedBucket, err
	}
	content, err := extractLogContent(file, handler)
	if err != nil {
		return "", nil, cfg_file, selectedBucket, err
	}
	defer file.Close()
	return content, &handler.Filename, cfg_file, selectedBucket, nil
}
func extractLogContent(file multipart.File, handler *multipart.FileHeader) (string, error) {
	if filepath.Ext(handler.Filename) != ".gz" && filepath.Ext(handler.Filename) != ".txt" {
		return "", errors.New("Invalid Format")
	}
	if filepath.Ext(handler.Filename) == ".gz" {
		gz, err := gzip.NewReader(file)
		if err != nil {
			return "", err
		}
		defer gz.Close()
		fContent := ""
		scanner := bufio.NewScanner(gz)
		for scanner.Scan() {
			fContent += scanner.Text()
			fContent += "\n"
		}
		return fContent, nil
	} else {
		data, err := ioutil.ReadAll(file)
		if err != nil {
			return "", nil
		}

		return string(data), nil
	}
}

package main

import (
	"encoding/json"
	"html/template"
	"net/http"
	"os"
	"radar-log-parser/go-app/report"
	"radar-log-parser/go-app/settings"
	"radar-log-parser/go-app/utilities"
	"sort"
	"strconv"
	"strings"
)

type Feedback struct {
	Error   bool
	Content string
}

var (
	feedBack       = Feedback{}
	fullLogDetails = report.FullDetails{}
	cfg_file       = report.Config{}
)

var (
	homeTempl             = template.Must(template.ParseFiles("templates/home.html"))
	upload_configTempl    = template.Must(template.ParseFiles("templates/upload_config_home.html"))
	edit_config_homeTempl = template.Must(template.ParseFiles("templates/editConfigHome.html"))
	edit_configTempl      = template.Must(template.ParseFiles("templates/editConfig.html"))
	delete_configTempl    = template.Must(template.ParseFiles("templates/deleteConfig.html"))
	feedbackTempl         = template.Must(template.ParseFiles("templates/feedback.html"))
	reportTempl           = template.Must(template.ParseFiles("templates/report.html"))
)
var (
	project_id           string   = "log-parser-278319"
	region_id            string   = "ue"
	app_specific_buckets []string = []string{"log-parser-278319.appspot.com", "staging.log-parser-278319.appspot.com", "us.artifacts.log-parser-278319.appspot.com"}
) //TODO: Put in a config file later
var cloudConfigs map[string][]string = make(map[string][]string)

var (
	cfg_edit    string
	bucket_edit string
)

func main() {
	port := os.Getenv("PORT")
	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir("assets"))
	mux.Handle("/assets/", http.StripPrefix("/assets/", fs))
	mux.HandleFunc("/", homeHandler)
	fillConfigMap()
	http.ListenAndServe(":"+port, mux)
}
func fillConfigMap() {
	buckets, err := utilities.GetBuckets(project_id)
	if err != nil {
		return
	}
	for _, bucket := range buckets {
		allow := true
		for _, buckt := range app_specific_buckets {
			if buckt == bucket {
				allow = false
				break
			}
		}
		if allow {
			cfg, err := utilities.GetConfigFiles(bucket)
			if err != nil {
				return
			}
			cloudConfigs[bucket] = cfg
		}
	}
}
func getFeedBack(err error, content string) {
	if err != nil {
		feedBack.Error = true
		feedBack.Content = err.Error()
	} else {
		feedBack.Error = false
		feedBack.Content = content
	}
}
func homeHandler(w http.ResponseWriter, r *http.Request) {
	page := r.URL.Path[len("/"):]
	if r.Method != http.MethodPost {
		if len(page) > 5 {
			if strings.Contains(page, "UploadConfig") {
				fillUploadCfgPage(w, r)
			} else if strings.Contains(page, "analyzeLog") { //to remove
				homeTempl.Execute(w, cloudConfigs)
			} else if strings.Contains(page, "editConfig") {
				edit_config_homeTempl.Execute(w, cloudConfigs)
			} else if strings.Contains(page, "deleteConfig") {
				delete_configTempl.Execute(w, cloudConfigs)
			} else {
				report.LogReport(w, r, &fullLogDetails, &cfg_file)
			}
		} else {
			homeTempl.Execute(w, cloudConfigs)
		}
		return
	}
	switch page {
	case "report/events/details":
		loadEventDetails(w, r, fullLogDetails.Analysis_details.RawLog)
	case "loglevel":
		loadLogLevel(w, r, fullLogDetails.Analysis_details.Platform, fullLogDetails.Analysis_details.RawLog)
	case "UploadConfig":
		loadUploadConfig(w, r)
	case "editConfig":
		loadEditConfig(w, r)
	case "deleteConfig":
		loadDeleteConfig(w, r)
	default:
		loadAnalyseLog(w, r, &fullLogDetails, &cfg_file)
	}

}
func fillUploadCfgPage(w http.ResponseWriter, r *http.Request) {
	bucketList := make([]string, 0, len(cloudConfigs))
	for k := range cloudConfigs {
		bucketList = append(bucketList, k)
	}
	upload_configTempl.Execute(w, bucketList)
}
func loadLogLevel(w http.ResponseWriter, r *http.Request, platform string, rawlog string) {
	r.ParseMultipartForm(10 << 20)
	level := r.FormValue("selectedLevel")
	logContent := report.GetLogLeveldetails(platform, level, rawlog)
	w.Write([]byte(logContent))
}
func loadUploadConfig(w http.ResponseWriter, r *http.Request) {
	configs, err := settings.UploadConfigFile(r, project_id, cloudConfigs)
	getFeedBack(err, "Upload Config")
	if err == nil {
		cloudConfigs = configs
	}
	cloudConfigs = configs
	feedbackTempl.Execute(w, feedBack)
}
func loadEditConfig(w http.ResponseWriter, r *http.Request) {
	if r.FormValue("action") == "Save" {
		err := settings.SaveConfig(r, bucket_edit, cfg_edit)
		getFeedBack(err, "Edit Config")
		feedbackTempl.Execute(w, feedBack)
	} else {
		bck, cfg, content, err := settings.DisplayConfig(w, r, project_id, region_id)
		bucket_edit = bck
		cfg_edit = cfg
		if err != nil {
			getFeedBack(err, "Edit Config")
			feedbackTempl.Execute(w, feedBack)
			return
		}
		edit_configTempl.Execute(w, content)
	}
}
func loadDeleteConfig(w http.ResponseWriter, r *http.Request) {
	configs, err := settings.DeleteConfig(r, project_id, region_id, cloudConfigs)
	getFeedBack(err, "Delete Config")
	if err == nil {
		cloudConfigs = configs
	}
	feedbackTempl.Execute(w, feedBack)
}
func loadAnalyseLog(w http.ResponseWriter, r *http.Request, fullDetails *report.FullDetails, conf_file *report.Config) {
	err := report.AnalyseLog(w, r, project_id, region_id, &fullLogDetails, &cfg_file)
	if err != nil {
		getFeedBack(err, "Log Analysis Error")
		feedbackTempl.Execute(w, feedBack)
		return
	}
	reportTempl.Execute(w, fullLogDetails.Analysis_details)
}
func loadEventDetails(w http.ResponseWriter, r *http.Request, rawlog string) {
	r.ParseMultipartForm(10 << 20)
	startIndex, _ := strconv.Atoi(r.FormValue("StartIndex"))
	endIndex, _ := strconv.Atoi(r.FormValue("EndIndex"))
	logs := strings.Split(rawlog, "\n")
	type Reponse struct {
		Content   []string
		Highlight map[int]bool //index=> true = must be highlight
	}
	event_content := []string{}
	highlight := map[int]bool{}
	if startIndex < 0 {
		startIndex = 0
	}
	if endIndex > len(logs) {
		endIndex = len(logs)
	}
	event_content = fillEventDetails(startIndex, endIndex, logs, event_content, highlight)
	resp := Reponse{
		Content:   event_content,
		Highlight: highlight,
	}
	jsonValue, _ := json.Marshal(resp)
	w.Write(jsonValue)
}
func fillEventDetails(startIndex int, endIndex int, logs []string, event_content []string, highlight map[int]bool) []string {
	order_ev_lines := make([]int, 0, len(fullLogDetails.ImportantEvents))
	for line, _ := range fullLogDetails.ImportantEvents {
		order_ev_lines = append(order_ev_lines, line)
	}
	sort.Ints(order_ev_lines)
	last_index := startIndex
	for _, index := range order_ev_lines {
		if index < startIndex {
			continue
		}
		if index <= endIndex {
			content_slice := logs[last_index:index]
			event_content = append(event_content, strings.Join(content_slice, "\n"))
			highlight[len(event_content)-1] = false
			event_content = append(event_content, logs[index])
			highlight[len(event_content)-1] = true
			last_index = index + 1
		} else {
			content_slice := logs[last_index : endIndex+1]
			event_content = append(event_content, strings.Join(content_slice, "\n"))
			highlight[len(event_content)-1] = false
			last_index = endIndex + 1
			break
		}
	}
	if last_index <= endIndex {
		content_slice := logs[last_index : endIndex+1]
		event_content = append(event_content, strings.Join(content_slice, "\n"))
		highlight[len(event_content)-1] = false
	}
	return event_content
}

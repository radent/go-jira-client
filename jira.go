package gojira

import (
	"compress/gzip"
   "crypto/tls"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"os"
	"errors"
	"bytes"
	"strings"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type Jira struct {
	BaseUrl      string
	ApiPath      string
	ActivityPath string
	Client       *http.Client
	Auth         *Auth
}

type Auth struct {
	Login    string
	Password string
}

type Version struct {
	Id        string       `json:"id,omitempty"`
	Self      string       `json:"self,omitempty"`
	Name      string       `json:"name,omitempty"`
	Description      string       `json:"description,omitempty"`
	Project      string       `json:"project,omitempty"`
	ProjectId  int `json:"projectId,omitempty"`
}

type Comment struct {
	Body      string       `json:"body,omitempty"`
}

type Pagination struct {
	Total      int
	StartAt    int
	MaxResults int
	Page       int
	PageCount  int
	Pages      []int
}

func (p *Pagination) Compute() {
	p.PageCount = int(math.Ceil(float64(p.Total) / float64(p.MaxResults)))
	p.Page = int(math.Ceil(float64(p.StartAt) / float64(p.MaxResults)))

	p.Pages = make([]int, p.PageCount)
	for i := range p.Pages {
		p.Pages[i] = i
	}
}

type IssueRef struct {
	Id        string       `json:"id,omitempty"`
	Key       string       `json:"key,omitempty"`
	Self      string       `json:"self,omitempty"`
}
type Issue struct {
	Id        string       `json:"id,omitempty"`
	Key       string       `json:"key,omitempty"`
	Self      string       `json:"self,omitempty"`
	Expand    string       `json:"expand,omitempty"`
	Fields    *IssueFields `json:"fields,omitempty"`
}

type SearchResult struct {
	Expand     string
	StartAt    int
	MaxResults int
	Total      int
	Issues     []*Issue
	Pagination *Pagination
}

type IssueFields struct {
	IssueType   *IssueType   `json:"issuetype,omitempty"`
	Summary     string       `json:"summary,omitempty"`
	Description string       `json:"description,omitempty"`
	Reporter    *User        `json:"reporter,omitempty"`
	Assignee    *User        `json:"assignee,omitempty"`
	Project     *JiraProject `json:"project,omitempty"`
	Created     string       `json:"created,omitempty"`
	Versions    []*Version	 `json:"versions,omitempty"`
	// ug. how do we make this generic?
	CrashReportId float32 `json:"customfield_10021,omitempty"`
	BacktraceHash string  `json:"customfield_10022,omitempty"`
	CrashCount	  float32 `json:"customfield_10023,omitempty"`
}

type IssueType struct {
	Self        string `json:"self,omitempty"`
	Id          string `json:"id,omitempty"`
	Description string `json:"description,omitempty"`
	IconUrl     string `json:"iconUrl,omitempty"`
	Name        string `json:"name,omitempty"`
	Subtask     bool   `json:"subTask,omitempty"`
}

type JiraProject struct {
	Self       string            `json:"self,omitempty"`
	Id         string            `json:"id,omitempty"`
	Key        string            `json:"key,omitempty"`
	Name       string            `json:"name,omitempty"`
	AvatarUrls map[string]string `json:"avatarUrls,omitempty"`
}

type ActivityItem struct {
	Title    string    `xml:"title"json:"title"`
	Id       string    `xml:"id"json:"id"`
	Link     []Link    `xml:"link"json:"link"`
	Updated  time.Time `xml:"updated"json:"updated"`
	Author   Person    `xml:"author"json:"author"`
	Summary  Text      `xml:"summary"json:"summary"`
	Category Category  `xml:"category"json:"category"`
}

type ActivityFeed struct {
	XMLName xml.Name        `xml:"http://www.w3.org/2005/Atom feed"json:"xml_name"`
	Title   string          `xml:"title"json:"title"`
	Id      string          `xml:"id"json:"id"`
	Link    []Link          `xml:"link"json:"link"`
	Updated time.Time       `xml:"updated,attr"json:"updated"`
	Author  Person          `xml:"author"json:"author"`
	Entries []*ActivityItem `xml:"entry"json:"entries"`
}

type Category struct {
	Term string `xml:"term,attr"json:"term"`
}

type Link struct {
	Rel  string `xml:"rel,attr,omitempty"json:"rel"`
	Href string `xml:"href,attr"json:"href"`
}

type Person struct {
	Name     string `xml:"name"json:"name"`
	URI      string `xml:"uri"json:"uri"`
	Email    string `xml:"email"json:"email"`
	InnerXML string `xml:",innerxml"json:"inner_xml"`
}

type Text struct {
	Type string `xml:"type,attr,omitempty"json:"type"`
	Body string `xml:",chardata"json:"body"`
}

func NewJira(baseUrl string, apiPath string, activityPath string, auth *Auth) *Jira {

   // Accept our invalid self-signed Jira certificate.
   tr := &http.Transport{
      TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
   }
	client := &http.Client{Transport:tr}

	return &Jira{
		BaseUrl:      baseUrl,
		ApiPath:      apiPath,
		ActivityPath: activityPath,
		Client:       client,
		Auth:         auth,
	}
}

const (
	DateLayout = "2006-01-02T15:04:05.000-0700"
)

func (j *Jira) buildAndExecRequest(method string, url string, body io.Reader) []byte {
	if body != nil {
		fo, err := os.Create("last_body.txt")
		if err != nil {
			panic("could not create last_response.txt")
		}
		defer fo.Close()
		body = io.TeeReader(body, fo)
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		panic("Error while building jira request")
	}
	if body != nil {
		req.Header.Add("Content-Type", "application/json;charset=UTF-8")
	}

	req.SetBasicAuth(j.Auth.Login, j.Auth.Password)

	resp, err := j.Client.Do(req)
   if err != nil {
      fmt.Printf("Request failed: %s", err.Error())
      return nil
   }
	defer resp.Body.Close()
	if resp.Header.Get("Content-Encoding") == "gzip" {
		resp.Body, err = gzip.NewReader(resp.Body)
		if err != nil {
			panic(err)
		}
		defer resp.Body.Close()
	}
	contents, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("%s", err)
	}

	fo, err := os.Create("last_response.txt")
	if err != nil {
		panic("could not create last_response.txt")
	}
	defer fo.Close()
	_, err = fo.Write(contents)
	// fmt.Printf("response\n%s\n", contents)

	return contents
}

func (j *Jira) UserActivity(user string) (ActivityFeed, error) {
	url := j.BaseUrl + j.ActivityPath + "?streams=" + url.QueryEscape("user IS "+user)

	return j.Activity(url)
}

func (j *Jira) Activity(url string) (ActivityFeed, error) {

	contents := j.buildAndExecRequest("GET", url, nil)

	var activity ActivityFeed
	err := xml.Unmarshal(contents, &activity)
	if err != nil {
		fmt.Println("%s", err)
	}

	return activity, err
}

// search issues assigned to given user
func (j *Jira) IssuesAssignedTo(user string, maxResults int, startAt int) SearchResult {

	url := j.BaseUrl + j.ApiPath + "/search?jql=assignee=\"" + url.QueryEscape(user) + "\"&startAt=" + strconv.Itoa(startAt) + "&maxResults=" + strconv.Itoa(maxResults)
	contents := j.buildAndExecRequest("GET", url, nil)

	var issues SearchResult
	err := json.Unmarshal(contents, &issues)
	if err != nil {
		fmt.Println("%s", err)
	}

	pagination := Pagination{
		Total:      issues.Total,
		StartAt:    issues.StartAt,
		MaxResults: issues.MaxResults,
	}
	pagination.Compute()

	issues.Pagination = &pagination

	return issues
}

// search an issue by its id
func (j *Jira) Issue(id string) Issue {

	url := j.BaseUrl + j.ApiPath + "/issue/" + id
	contents := j.buildAndExecRequest("GET", url, nil)

	var issue Issue
	err := json.Unmarshal(contents, &issue)
	if err != nil {
		fmt.Println("%s", err)
	}

	return issue
}

func (j *Jira) SaveIssue(issue *Issue) (error) {
	// zero out these values so they don't get pushed
	issue.Fields.Reporter = nil
	issue.Fields.Assignee = nil
	issue.Fields.Created = ""

	encoded, err := json.MarshalIndent(issue, "", "   ")
	if err != nil {
		return err
	}
	//fmt.Printf("saving -> %s", encoded)

	body := bytes.NewBuffer(encoded)
	url := j.BaseUrl + j.ApiPath + "/issue/" + issue.Key
	contents := j.buildAndExecRequest("PUT", url, body)

	if len(contents) > 0 {
	   return errors.New(fmt.Sprintf("error: %s", contents))
	}
   return nil
}

func NewIssue(project string, issue_type string) (*Issue) {
		issue := Issue{
			Fields : &IssueFields{
				IssueType: &IssueType{Name: issue_type},
				Project: &JiraProject{Key: project},
		      Versions: []*Version{},
			},
		}
		return &issue
}

// create a new issue an issue by its id
func (j *Jira) CreateIssue(issue *Issue) (*IssueRef, error) {
	encoded, err := json.MarshalIndent(issue, "", "   ")
	if err != nil {
		return nil, err
	}
	body := bytes.NewBuffer(encoded)
	url := j.BaseUrl + j.ApiPath + "/issue/"
	contents := j.buildAndExecRequest("POST", url, body)

	var result IssueRef
   fmt.Printf("unmarshalling...%s\n", contents)
	err = json.Unmarshal(contents, &result)
	if err != nil {
		return nil, err
	}
	fmt.Printf("done!! %s...\n", result)
	return &result, nil
}

func (j *Jira) Search(jql string, maxResults int) (SearchResult, error) {
	url := j.BaseUrl + j.ApiPath + "/search?jql=" + url.QueryEscape(jql) + "&maxResults=" + strconv.Itoa(maxResults)

	contents := j.buildAndExecRequest("GET", url, nil)

	var issues SearchResult
	//fmt.Printf("unmarshalling...%s\n", contents)
	err := json.Unmarshal(contents, &issues)
	if err != nil {
		return issues, err
	}
	// fmt.Printf("done!! %s...\n", issues)
	return issues, nil
}

func (j *Jira) AddComment(issue_key string, comment *Comment) (*Comment, error) {
	var result Comment

	encoded, err := json.MarshalIndent(comment, "", "   ")
	if err != nil {
		log.Printf("error: %s\n", err)
		return nil, err
	}
	body := bytes.NewBuffer(encoded)
	url := j.BaseUrl + j.ApiPath + "/issue/" + issue_key + "/comment"
	contents := j.buildAndExecRequest("POST", url, body)

	fmt.Printf("unmarshalling...%s\n", contents)
	err = json.Unmarshal(contents, &result)
	if err != nil {
		log.Printf("error: %s\n", err)
		return nil, err
	}
	// fmt.Printf("done!! %s...\n", result)
	return &result, nil
}

func (j *Jira) GetAllVersions(productKey string) ([]*Version, error) {
	url := j.BaseUrl + j.ApiPath + "/project/" + productKey + "/versions"

	contents := j.buildAndExecRequest("GET", url, nil)

	var result []*Version;
	// fmt.Printf("unmarshalling...\n")
	err := json.Unmarshal(contents, &result)
	if err != nil {
		return result, err
	}
	// fmt.Printf("done!! %s...\n", issues)
	return result, nil
}


func (j *Jira) AddVersionToIssue(issue *IssueRef, version *Version) (error) {
	encoded := fmt.Sprintf(`
		{
			"update" : {
				"versions" : [ { "add" : { "id" : "%s" } } ]
			}
		}
	`, version.Id)

	body := strings.NewReader(encoded)
	url := j.BaseUrl + j.ApiPath + "/issue/" + issue.Key
	contents := j.buildAndExecRequest("PUT", url, body)

	if len(contents) == 0 {
		return nil
	}
	return errors.New(fmt.Sprintf("error: %s", contents))
}

func (j *Jira) CreateVersion(version *Version) (*Version, error) {
	var result Version

	encoded, err := json.MarshalIndent(version, "", "   ")
	if err != nil {
		return nil, err
	}
	body := bytes.NewBuffer(encoded)
	url := j.BaseUrl + j.ApiPath + "/version/"
	contents := j.buildAndExecRequest("POST", url, body)

	// fmt.Printf("unmarshalling...\n")
	err = json.Unmarshal(contents, &result)
	if err != nil {
		return nil, err
	}
	// fmt.Printf("done!! %s...\n", result)
	return &result, nil
}

func (f *IssueFields) AddVersion(version *Version) {
	if (f.Versions == nil) {
		f.Versions = []*Version{}
	}
	for _, v := range f.Versions {
		if v.Name == version.Name {
			return
		}
	}
	f.Versions = append(f.Versions, version)
}


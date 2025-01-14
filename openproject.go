package openproject

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/google/go-querystring/query"
	"github.com/pkg/errors"
)

// OPGenericDescription is an structure widely used in several OpenProject API objects
type OPGenericDescription struct {
	Format string `json:"format,omitempty" structs:"format,omitempty"`
	Raw    string `json:"raw,omitempty" structs:"raw,omitempty"`
	HTML   string `json:"html,omitempty" structs:"html,omitempty"`
}

// Time represents the Time definition of OpenProject as a time.Time of go
type Time time.Time

// Equal compares time
func (t Time) Equal(u Time) bool {
	return time.Time(t).Equal(time.Time(u))
}

// Date represents the Date definition of OpenProject as a time.Time of go
type Date time.Time

// httpClient defines an interface for an http.Client implementation
type httpClient interface {
	Do(request *http.Request) (response *http.Response, err error)
}

// Client manages communication with the OpenProject API.
type Client struct {
	// HTTP client used to communicate with the API.
	client httpClient

	// Base URL for API requests.
	baseURL *url.URL

	// Session storage if the user authenticates with Session cookies
	session *Session

	// Services used for talking to different parts of OpenProject API.
	Authentication *AuthenticationService
	WorkPackage    *WorkPackageService
	Project        *ProjectService
	User           *UserService
	Status         *StatusService
	WikiPage       *WikiPageService
	Attachment     *AttachmentService
	Category       *CategoryService
	Query          *QueryService
}

// NewClient returns a new OpenProject API client.
// If a nil httpClient is provided, http.DefaultClient will be used.
func NewClient(httpClient httpClient, baseURL string) (*Client, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	// ensure the baseURL contains a trailing slash so that all paths are preserved in later calls
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}

	parsedBaseURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}

	c := &Client{
		client:  httpClient,
		baseURL: parsedBaseURL,
	}
	c.Authentication = &AuthenticationService{client: c}
	c.WorkPackage = &WorkPackageService{client: c}
	c.Project = &ProjectService{client: c}
	c.User = &UserService{client: c}
	c.Status = &StatusService{client: c}
	c.WikiPage = &WikiPageService{client: c}
	c.Attachment = &AttachmentService{client: c}
	c.Category = &CategoryService{client: c}
	c.Query = &QueryService{client: c}

	return c, nil
}

// NewRawRequestWithContext creates an API request.
// A relative URL can be provided in urlStr, in which case it is resolved relative to the baseURL of the Client.
// Allows using an optional native io.Reader for sourcing the request body.
func (c *Client) NewRawRequestWithContext(ctx context.Context, method, urlStr string, body io.Reader) (*http.Request, error) {
	rel, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	// Relative URLs should be specified without a preceding slash since baseURL will have the trailing slash
	rel.Path = strings.TrimLeft(rel.Path, "/")

	u := c.baseURL.ResolveReference(rel)

	req, err := newRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	// Set authentication information
	if c.Authentication.authType == authTypeSession {
		// Set session cookie if there is one
		if c.session != nil {
			for _, cookie := range c.session.Cookies {
				req.AddCookie(cookie)
			}
		}
	} else if c.Authentication.authType == authTypeBasic {
		// Set basic auth information
		if c.Authentication.username != "" {
			req.SetBasicAuth(c.Authentication.username, c.Authentication.password)
		}
	}

	return req, nil
}

// NewRawRequest wraps NewRawRequestWithContext using the background context.
func (c *Client) NewRawRequest(method, urlStr string, body io.Reader) (*http.Request, error) {
	return c.NewRawRequestWithContext(context.Background(), method, urlStr, body)
}

// NewRequestWithContext creates an API request.
// A relative URL can be provided in urlStr, in which case it is resolved relative to the baseURL of the Client.
// If specified, the value pointed to by body is JSON encoded and included as the request body.
func (c *Client) NewRequestWithContext(ctx context.Context, method, urlStr string, body interface{}) (*http.Request, error) {
	rel, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	// Relative URLs should be specified without a preceding slash since baseURL will have the trailing slash
	rel.Path = strings.TrimLeft(rel.Path, "/")

	u := c.baseURL.ResolveReference(rel)

	var buf io.ReadWriter
	if body != nil {
		buf = new(bytes.Buffer)
		err = json.NewEncoder(buf).Encode(body)
		if err != nil {
			return nil, err
		}
	}

	req, err := newRequestWithContext(ctx, method, u.String(), buf)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	// Set authentication information
	if c.Authentication.authType == authTypeSession {
		// Set session cookie if there is one
		if c.session != nil {
			for _, cookie := range c.session.Cookies {
				req.AddCookie(cookie)
			}
		}
	} else if c.Authentication.authType == authTypeBasic {
		// Set basic auth information
		if c.Authentication.username != "" {
			req.SetBasicAuth(c.Authentication.username, c.Authentication.password)
		}
	}

	return req, nil
}

// NewRequest wraps NewRequestWithContext using the background context.
func (c *Client) NewRequest(method, urlStr string, body interface{}) (*http.Request, error) {
	return c.NewRequestWithContext(context.Background(), method, urlStr, body)
}

// addOptions adds the parameters in opt as URL query parameters to s.  opt
// must be a struct whose fields may contain "url" tags.
func addOptions(s string, opt interface{}) (string, error) {
	v := reflect.ValueOf(opt)
	if v.Kind() == reflect.Ptr && v.IsNil() {
		return s, nil
	}

	u, err := url.Parse(s)
	if err != nil {
		return s, err
	}

	qs, err := query.Values(opt)
	if err != nil {
		return s, err
	}

	u.RawQuery = qs.Encode()
	return u.String(), nil
}

// NewMultiPartRequestWithContext creates an API request including a multi-part file.
// A relative URL can be provided in urlStr, in which case it is resolved relative to the baseURL of the Client.
// If specified, the value pointed to by buf is a multipart form.
func (c *Client) NewMultiPartRequestWithContext(ctx context.Context, method, urlStr string, buf *bytes.Buffer) (*http.Request, error) {
	rel, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	// Relative URLs should be specified without a preceding slash since baseURL will have the trailing slash
	rel.Path = strings.TrimLeft(rel.Path, "/")

	u := c.baseURL.ResolveReference(rel)

	req, err := newRequestWithContext(ctx, method, u.String(), buf)
	if err != nil {
		return nil, err
	}

	// Set authentication information
	if c.Authentication.authType == authTypeSession {
		// Set session cookie if there is one
		if c.session != nil {
			for _, cookie := range c.session.Cookies {
				req.AddCookie(cookie)
			}
		}
	} else if c.Authentication.authType == authTypeBasic {
		// Set basic auth information
		if c.Authentication.username != "" {
			req.SetBasicAuth(c.Authentication.username, c.Authentication.password)
		}
	}

	return req, nil
}

// NewMultiPartRequest wraps NewMultiPartRequestWithContext using the background context.
func (c *Client) NewMultiPartRequest(method, urlStr string, buf *bytes.Buffer) (*http.Request, error) {
	return c.NewMultiPartRequestWithContext(context.Background(), method, urlStr, buf)
}

// Do sends an API request and returns the API response.
// The API response is JSON decoded and stored in the value pointed to by v, or returned as an error if an API error has occurred.
func (c *Client) Do(req *http.Request, v interface{}) (*Response, error) {
	httpResp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}

	// requestDump, err := httputil.DumpResponse(httpResp, true)
	// if err != nil {
	// 	fmt.Println(err.Error())
	// } else {
	// 	fmt.Println(requestDump)
	// }

	err = CheckResponse(httpResp)
	if err != nil {
		// In case of error we still return the response
		return newResponse(httpResp, nil), err
	}

	if v != nil {
		// Open a NewDecoder and defer closing the reader only if there is a provided interface to decode to
		defer httpResp.Body.Close()
		err = json.NewDecoder(httpResp.Body).Decode(v)
	}

	resp := newResponse(httpResp, v)
	return resp, err
}

// Download request a file download
func (c *Client) Download(req *http.Request) (*http.Response, error) {
	httpResp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}

	// requestDump, err := httputil.DumpResponse(httpResp, true)
	// if err != nil {
	// 	fmt.Println(err.Error())
	// } else {
	// 	fmt.Println(requestDump)
	// }

	err = CheckResponse(httpResp)

	return httpResp, err
}

// CheckResponse checks the API response for errors, and returns them if present.
// A response is considered an error if it has a status code outside the 200 range.
// The caller is responsible to analyze the response body.
func CheckResponse(r *http.Response) error {
	if c := r.StatusCode; 200 <= c && c <= 299 {
		return nil
	}

	err := fmt.Errorf("request failed. Please analyze the request body for more details. Status code: %d", r.StatusCode)
	return err
}

// GetBaseURL will return you the Base URL.
// This is the same URL as in the NewClient constructor
func (c *Client) GetBaseURL() url.URL {
	return *c.baseURL
}

// Response represents OpenProject API response. It wraps http.Response returned from
// API and provides information about paging.
type Response struct {
	*http.Response
	Total    int
	Count    int
	PageSize int
	Offset   int
}

// New response
func newResponse(r *http.Response, v interface{}) *Response {
	resp := &Response{Response: r}
	resp.populatePageValues(v)
	return resp
}

// Sets paging values if response json was parsed to searchResult type
// (can be extended with other types if they also need paging info)
// TODO: Improve implementation to avoid redundancy without losing efficiency (reflect alternative is not efficient)
func (r *Response) populatePageValues(v interface{}) {
	switch value := v.(type) {
	case *SearchResultWP:
		r.Total = value.Total
		r.Count = value.Count
		r.PageSize = value.PageSize
		r.Offset = value.Offset
	case *SearchResultUser:
		r.Total = value.Total
		r.Count = value.Count
		r.PageSize = value.PageSize
		r.Offset = value.Offset
	case *SearchResultQuery:
		r.Total = value.Total
		r.Count = value.Count
		r.PageSize = value.PageSize
		r.Offset = value.Offset
	}
}

// BasicAuthTransport is an http.RoundTripper that authenticates all requests
// using HTTP Basic Authentication with the provided username and password.
type BasicAuthTransport struct {
	Username string
	Password string

	// Transport is the underlying HTTP transport to use when making requests.
	// It will default to http.DefaultTransport if nil.
	Transport http.RoundTripper
}

// RoundTrip implements the RoundTripper interface.  We just add the
// basic auth and return the RoundTripper for this transport type.
func (t *BasicAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := cloneRequest(req) // per RoundTripper contract

	req2.SetBasicAuth(t.Username, t.Password)
	return t.transport().RoundTrip(req2)
}

// Client returns an *http.Client that makes requests that are authenticated
// using HTTP Basic Authentication.  This is a nice little bit of sugar
// so we can just get the client instead of creating the client in the calling code.
// If it's necessary to send more information on client init, the calling code can
// always skip this and set the transport itself.
func (t *BasicAuthTransport) Client() *http.Client {
	return &http.Client{Transport: t}
}

// Transport
func (t *BasicAuthTransport) transport() http.RoundTripper {
	if t.Transport != nil {
		return t.Transport
	}
	return http.DefaultTransport
}

// CookieAuthTransport is an http.RoundTripper that authenticates all requests
// using cookie-based authentication.
// Note that it is generally preferable to use HTTP BASIC authentication with the REST API.
type CookieAuthTransport struct {
	Username string
	Password string
	AuthURL  string

	// SessionObject is the authenticated cookie string.s
	// It's passed in each call to prove the client is authenticated.
	SessionObject []*http.Cookie

	// Transport is the underlying HTTP transport to use when making requests.
	// It will default to http.DefaultTransport if nil.
	Transport http.RoundTripper
}

// RoundTrip adds the session object to the request.
func (t *CookieAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.SessionObject == nil {
		err := t.setSessionObject()
		if err != nil {
			return nil, errors.Wrap(err, "cookieauth: no session object has been set")
		}
	}

	req2 := cloneRequest(req) // per RoundTripper contract
	for _, cookie := range t.SessionObject {
		// Don't add an empty value cookie to the request
		if cookie.Value != "" {
			req2.AddCookie(cookie)
		}
	}

	return t.transport().RoundTrip(req2)
}

// Client returns an *http.Client that makes requests that are authenticated
// using cookie authentication
func (t *CookieAuthTransport) Client() *http.Client {
	return &http.Client{Transport: t}
}

// setSessionObject attempts to authenticate the user and set
// the session object (e.g. cookie)
func (t *CookieAuthTransport) setSessionObject() error {
	req, err := t.buildAuthRequest()
	if err != nil {
		return err
	}

	var authClient = &http.Client{
		Timeout: time.Second * 60,
	}
	resp, err := authClient.Do(req)
	if err != nil {
		return err
	}

	t.SessionObject = resp.Cookies()
	return nil
}

// getAuthRequest assembles the request to get the authenticated cookie
func (t *CookieAuthTransport) buildAuthRequest() (*http.Request, error) {
	body := struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}{
		t.Username,
		t.Password,
	}

	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(body)

	req, err := http.NewRequest("POST", t.AuthURL, b)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func (t *CookieAuthTransport) transport() http.RoundTripper {
	if t.Transport != nil {
		return t.Transport
	}
	return http.DefaultTransport
}

// JWTAuthTransport is an http.RoundTripper that authenticates all requests
// using JWT based authentication.
// NOTE: this form of auth should be used by add-ons installed from the Atlassian marketplace.
type JWTAuthTransport struct {
	Secret []byte
	Issuer string

	// Transport is the underlying HTTP transport to use when making requests.
	// It will default to http.DefaultTransport if nil.
	Transport http.RoundTripper
}

// Client JWTAuthTransport
func (t *JWTAuthTransport) Client() *http.Client {
	return &http.Client{Transport: t}
}

// transport JWTAuthTransport
func (t *JWTAuthTransport) transport() http.RoundTripper {
	if t.Transport != nil {
		return t.Transport
	}
	return http.DefaultTransport
}

// RoundTrip adds the session object to the request.
func (t *JWTAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := cloneRequest(req) // per RoundTripper contract
	exp := time.Duration(59) * time.Second
	qsh := t.createQueryStringHash(req.Method, req2.URL)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss": t.Issuer,
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(exp).Unix(),
		"qsh": qsh,
	})

	jwtStr, err := token.SignedString(t.Secret)
	if err != nil {
		return nil, errors.Wrap(err, "jwtAuth: error signing JWT")
	}

	req2.Header.Set("Authorization", fmt.Sprintf("JWT %s", jwtStr))
	return t.transport().RoundTrip(req2)
}

// CreateQueryStringHash
func (t *JWTAuthTransport) createQueryStringHash(httpMethod string, openprojURL *url.URL) string {
	canonicalRequest := t.canonicalizeRequest(httpMethod, openprojURL)
	h := sha256.Sum256([]byte(canonicalRequest))
	return hex.EncodeToString(h[:])
}

// CanonicalizeRequest
func (t *JWTAuthTransport) canonicalizeRequest(httpMethod string, openprojURL *url.URL) string {
	path := "/" + strings.Replace(strings.Trim(openprojURL.Path, "/"), "&", "%26", -1)

	var canonicalQueryString []string
	for k, v := range openprojURL.Query() {
		if k == "jwt" {
			continue
		}
		param := url.QueryEscape(k)
		value := url.QueryEscape(strings.Join(v, ""))
		canonicalQueryString = append(canonicalQueryString, strings.Replace(strings.Join([]string{param, value}, "="), "+", "%20", -1))
	}
	sort.Strings(canonicalQueryString)
	return fmt.Sprintf("%s&%s&%s", strings.ToUpper(httpMethod), path, strings.Join(canonicalQueryString, "&"))
}

// cloneRequest returns a clone of the provided *http.Request.
// The clone is a shallow copy of the struct and its Header map.
func cloneRequest(r *http.Request) *http.Request {
	// shallow copy of the struct
	r2 := new(http.Request)
	*r2 = *r
	// deep copy of the Header
	r2.Header = make(http.Header, len(r.Header))
	for k, s := range r.Header {
		r2.Header[k] = append([]string(nil), s...)
	}
	return r2
}

// Interpret Operator collection and return its string ( Used in searches like GetList(...) )
func interpretOperator(operator SearchOperator) string {
	result := "="

	switch operator {
	case GreaterThan:
		result = ">"
	case LowerThan:
		result = "<"
	case Different:
		result = "<>"
	}
	//TODO: Complete list of operators

	return result
}

// getObjectAndClient gets an inputObject (inputObject is an OpenProject object like WorkPackage, WikiPage, Status, etc.)
// and return a pointer to its Client from its service and an instance of the object itself
func getObjectAndClient(inputObj interface{}) (client *Client, resultObj interface{}) {
	switch inputObj.(type) {
	case *AttachmentService:
		client = inputObj.(*AttachmentService).client
		resultObj = new(Attachment)
	case *CategoryService:
		client = inputObj.(*CategoryService).client
		resultObj = new(Category)
	case *ProjectService:
		client = inputObj.(*ProjectService).client
		resultObj = new(Project)
	case *QueryService:
		client = inputObj.(*QueryService).client
		resultObj = new(Query)
	case *StatusService:
		client = inputObj.(*StatusService).client
		resultObj = new(Status)
	case *UserService:
		client = inputObj.(*UserService).client
		resultObj = new(User)
	case *WikiPageService:
		client = inputObj.(*WikiPageService).client
		resultObj = new(WikiPage)
	case *WorkPackageService:
		client = inputObj.(*WorkPackageService).client
		resultObj = new(WorkPackage)
	}

	return client, resultObj
}

// getObjectAndClient gets an inputObject (inputObject is an OpenProject object like WorkPackage, WikiPage, Status, etc.)
// and return a pointer to its Client from its service and an instance of the ObjectList
func getObjectListAndClient(inputObj interface{}) (client *Client, resultObjList interface{}) {
	switch inputObj.(type) {
	case *AttachmentService:
		client = inputObj.(*AttachmentService).client
		// TODO implement
	case *CategoryService:
		client = inputObj.(*CategoryService).client
		resultObjList = new(CategoryList)
	case *ProjectService:
		client = inputObj.(*ProjectService).client
		resultObjList = new(SearchResultProject)
	case *QueryService:
		client = inputObj.(*QueryService).client
		resultObjList = new(SearchResultQuery)
	case *StatusService:
		client = inputObj.(*StatusService).client
		resultObjList = new(SearchResultStatus)
	case *UserService:
		client = inputObj.(*UserService).client
		resultObjList = new(SearchResultUser)
	// WikiPage endpoint does not support POST action
	// case *WikiPageService:
	case *WorkPackageService:
		client = inputObj.(*WorkPackageService).client
		resultObjList = new(SearchResultWP)
	}

	return client, resultObjList
}

// GetWithContext (generic) retrieves object (HTTP GET verb)
// obj can be any main object (attachment, user, project, work-package, etc...) as well as response interface{}
func GetWithContext(ctx context.Context, objService interface{}, apiEndPoint string) (interface{}, *Response, error) {
	client, resultObj := getObjectAndClient(objService)
	apiEndPoint = strings.TrimRight(apiEndPoint, "/")
	if client == nil {
		return nil, nil, errors.New("Null client, object not identified")
	}

	req, err := client.NewRequestWithContext(ctx, "GET", apiEndPoint, nil)
	if err != nil {
		return nil, nil, err
	}

	resp, err := client.Do(req, resultObj)

	if err != nil {
		return nil, resp, NewOpenProjectError(resp, err)
	}
	return resultObj, resp, nil
}

// GetListWithContext (generic) retrieves list of objects (HTTP GET verb)
// obj list is a collection of any main object (attachment, user, project, work-package, etc...) as well as response interface{}
func GetListWithContext(ctx context.Context, objService interface{}, apiEndPoint string, options *FilterOptions) (interface{}, *Response, error) {
	client, resultObjList := getObjectListAndClient(objService)
	apiEndPoint = strings.TrimRight(apiEndPoint, "/")
	req, err := client.NewRequestWithContext(ctx, "GET", apiEndPoint, nil)
	if err != nil {
		return nil, nil, err
	}

	if options != nil {
		values := options.prepareFilters()
		req.URL.RawQuery = values.Encode()
	}

	resp, err := client.Do(req, resultObjList)
	if err != nil {
		oerr := NewOpenProjectError(resp, err)
		return nil, resp, oerr
	}

	return resultObjList, resp, nil
}

// CreateWithContext (generic) creates an instance af an object (HTTP POST verb)
// Return the instance of the object rendered into proper struct as interface{} to be cast in the caller
func CreateWithContext(ctx context.Context, objService interface{}, apiEndPoint string) (interface{}, *Response, error) {
	client, resultObj := getObjectAndClient(objService)
	req, err := client.NewRequestWithContext(ctx, "POST", apiEndPoint, resultObj)
	if err != nil {
		return nil, nil, err
	}
	resp, err := client.Do(req, nil)
	if err != nil {
		// incase of error return the resp for further inspection
		return nil, resp, err
	}

	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, resp, fmt.Errorf("could not read the returned data")
	}
	err = json.Unmarshal(data, resultObj)
	if err != nil {
		return nil, resp, fmt.Errorf("could not unmarshall the data into struct")
	}
	return resultObj, resp, nil
}

// DeleteWithContext (generic) retrieves object (HTTP DELETE verb)
// obj can be any main object (attachment, user, project, work-package, etc...)
func DeleteWithContext(ctx context.Context, objService interface{}, apiEndPoint string) (*Response, error) {
	client, _ := getObjectAndClient(objService)
	apiEndPoint = strings.TrimRight(apiEndPoint, "/")
	req, err := client.NewRequestWithContext(ctx, "DELETE", apiEndPoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req, nil)
	return resp, err
}

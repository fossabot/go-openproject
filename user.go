package openproject

import (
	"context"
	"fmt"
	"net/url"
)

/**
UserService handles users for the OpenProject instance / API.
*/
type UserService struct {
	client *Client
}

/**
User is the object representing OpenProject users.
TODO: Complete object with fields identityUrl, language, _links
*/
type User struct {
	Type      string `json:"_type,omitempty" structs:"_type,omitempty"`
	Id        int    `json:"id,omitempty" structs:"id,omitempty"`
	Name      string `json:"name,omitempty" structs:"name,omitempty"`
	CreatedAt *Time  `json:"createdAt,omitempty" structs:"createdAt,omitempty"`
	UpdatedAt *Time  `json:"updatedAt,omitempty" structs:"updatedAt,omitempty"`
	Login     string `json:"login,omitempty" structs:"login,omitempty"`
	Admin     bool   `json:"admin,omitempty" structs:"admin,omitempty"`
	FirstName string `json:"firstName,omitempty" structs:"firstName,omitempty"`
	lastName  string `json:"lastName,omitempty" structs:"lastName,omitempty"`
	Email     string `json:"email,omitempty" structs:"email,omitempty"`
	Avatar    string `json:"avatar,omitempty" structs:"avatar,omitempty"`
	Status    string `json:"status,omitempty" structs:"status,omitempty"`
}

/**
searchResult is only a small wrapper around the Search
*/
type searchResultUser struct {
	Embedded searchEmbeddedUser `json:"_embedded" structs:"_embedded"`
	Total    int                `json:"total" structs:"total"`
	Count    int                `json:"count" structs:"count"`
	PageSize int                `json:"pageSize" structs:"pageSize"`
	Offset   int                `json:"offset" structs:"offset"`
}

type searchEmbeddedUser struct {
	Elements []User `json:"elements" structs:"elements"`
}

/**
GetWithContext gets user info from OpenProject using its Account Id
// TODO: Implement GetList and adapt tests
*/
func (s *UserService) GetWithContext(ctx context.Context, accountId string) (*User, *Response, error) {
	apiEndpoint := fmt.Sprintf("api/v3/users?id=%s", accountId)
	req, err := s.client.NewRequestWithContext(ctx, "GET", apiEndpoint, nil)
	if err != nil {
		return nil, nil, err
	}

	user := new(User)
	resp, err := s.client.Do(req, user)
	if err != nil {
		return nil, resp, NewOpenProjectError(resp, err)
	}
	return user, resp, nil
}

/**
Get wraps GetWithContext using the background context.
*/
func (s *UserService) Get(accountId string) (*User, *Response, error) {
	return s.GetWithContext(context.Background(), accountId)
}

/**
GetListWithContext will retrieve a list of users using filters
*/
func (s *UserService) GetListWithContext(ctx context.Context, options *FilterOptions) ([]User, *Response, error) {
	u := url.URL{
		Path: "api/v3/users",
	}

	req, err := s.client.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return []User{}, nil, err
	}

	if options != nil {
		values := options.prepareFilters()
		req.URL.RawQuery = values.Encode()
	}

	v := new(searchResultUser)
	resp, err := s.client.Do(req, v)
	if err != nil {
		err = NewOpenProjectError(resp, err)
	}
	return v.Embedded.Elements, resp, err
}

/**
GetList wraps GetListWithContext using the background context.
*/
func (s *UserService) GetList(options *FilterOptions) ([]User, *Response, error) {
	return s.GetListWithContext(context.Background(), options)
}

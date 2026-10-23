package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/go-oauth2/oauth2/v4"
	"github.com/go-oauth2/oauth2/v4/models"
)

type clientRecord struct {
	ID          string   `json:"client_id"`
	Name        string   `json:"client_name"`
	Redirects   []string `json:"redirect_uris"`
	AuthMethod  string   `json:"token_endpoint_auth_method"`
	AuthMethods []string `json:"token_endpoint_auth_methods_supported,omitempty"`
}
type tokenRecord struct {
	Token       models.Token
	AccessHash  string
	RefreshHash string
}
type oauthData struct {
	Version int
	Issuer  string
	Clients map[string]clientRecord
	Tokens  map[string]tokenRecord
}
type oauthStore struct {
	path string
	data oauthData
}

func newOAuthStore(path, issuer string) (*oauthStore, error) {
	s := &oauthStore{path: path, data: oauthData{Version: 1, Issuer: issuer, Clients: map[string]clientRecord{}, Tokens: map[string]tokenRecord{}}}
	b, e := os.ReadFile(path)
	if errors.Is(e, os.ErrNotExist) {
		return s, nil
	}
	if e != nil {
		return nil, e
	}
	if json.Unmarshal(b, &s.data) != nil || s.data.Version != 1 || s.data.Issuer != issuer || s.data.Clients == nil || s.data.Tokens == nil {
		return nil, errors.New("saved cloud authorization is invalid or belongs to another hostname")
	}
	return s, nil
}
func (s *oauthStore) save() error { return atomicJSON(s.path, s.data) }
func (s *oauthStore) GetByID(_ context.Context, id string) (oauth2.ClientInfo, error) {
	c, ok := s.data.Clients[id]
	if !ok {
		return nil, errors.New("unregistered client")
	}
	return &models.Client{ID: id, Public: true, Domain: strings.Join(c.Redirects, "\n")}, nil
}
func (s *oauthStore) Create(_ context.Context, t oauth2.TokenInfo) error {
	b, e := json.Marshal(t)
	if e != nil {
		return e
	}
	var v models.Token
	if e = json.Unmarshal(b, &v); e != nil {
		return e
	}
	rec := tokenRecord{Token: v, AccessHash: fingerprint(v.Access), RefreshHash: fingerprint(v.Refresh)}
	rec.Token.Code = ""
	rec.Token.Access = ""
	rec.Token.Refresh = ""
	if rec.Token.Extension == nil {
		rec.Token.Extension = map[string][]string{}
	}
	keys := []string{}
	if v.Code != "" {
		keys = append(keys, "c:"+fingerprint(v.Code))
	}
	if v.Access != "" {
		keys = append(keys, "a:"+fingerprint(v.Access))
	}
	if v.Refresh != "" {
		keys = append(keys, "r:"+fingerprint(v.Refresh))
	}
	for _, key := range keys {
		s.data.Tokens[key] = rec
	}
	if e = s.save(); e != nil {
		for _, key := range keys {
			delete(s.data.Tokens, key)
		}
		return e
	}
	return nil
}
func tokenKey(kind, token string) string {
	if strings.HasPrefix(token, "digest:") {
		return kind + strings.TrimPrefix(token, "digest:")
	}
	return kind + fingerprint(token)
}
func (s *oauthStore) remove(kind, token string) error {
	key := tokenKey(kind, token)
	old, ok := s.data.Tokens[key]
	delete(s.data.Tokens, key)
	if e := s.save(); e != nil {
		if ok {
			s.data.Tokens[key] = old
		}
		return e
	}
	return nil
}
func (s *oauthStore) RemoveByCode(_ context.Context, t string) error    { return s.remove("c:", t) }
func (s *oauthStore) RemoveByAccess(_ context.Context, t string) error  { return s.remove("a:", t) }
func (s *oauthStore) RemoveByRefresh(_ context.Context, t string) error { return s.remove("r:", t) }
func (s *oauthStore) get(kind, token string) (oauth2.TokenInfo, error) {
	rec, ok := s.data.Tokens[kind+fingerprint(token)]
	if !ok {
		return nil, errors.New("invalid credential")
	}
	v := rec.Token
	b, _ := json.Marshal(v.Extension)
	json.Unmarshal(b, &v.Extension)
	if kind == "c:" {
		v.Code = token
	}
	if kind == "a:" {
		v.Access = token
	}
	if kind == "r:" {
		v.Refresh = token
		v.Access = "digest:" + rec.AccessHash
	}
	return &v, nil
}
func (s *oauthStore) GetByCode(_ context.Context, t string) (oauth2.TokenInfo, error) {
	return s.get("c:", t)
}
func (s *oauthStore) GetByAccess(_ context.Context, t string) (oauth2.TokenInfo, error) {
	return s.get("a:", t)
}
func (s *oauthStore) GetByRefresh(_ context.Context, t string) (oauth2.TokenInfo, error) {
	return s.get("r:", t)
}

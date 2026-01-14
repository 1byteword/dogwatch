package sso

import (
	"compress/flate"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"time"
)

var (
	ErrSAMLNotConfigured = errors.New("saml not configured")
	ErrSAMLInvalidConfig = errors.New("invalid saml configuration")
	ErrSAMLResponseInvalid = errors.New("invalid saml response")
	ErrSAMLAssertionExpired = errors.New("saml assertion expired")
)

// SAMLConfig holds SAML configuration for an organization
type SAMLConfig struct {
	Enabled          bool   `json:"enabled"`
	EntityID         string `json:"entity_id"`          // Our SP entity ID
	SSOURL           string `json:"sso_url"`            // IdP SSO URL
	SLOURL           string `json:"slo_url,omitempty"`  // IdP SLO URL (optional)
	Certificate      string `json:"certificate"`        // IdP certificate (PEM format)
	SigningKey       string `json:"-"`                  // Our signing key (PEM format)
	SigningCert      string `json:"signing_cert"`       // Our signing cert (PEM format)
	NameIDFormat     string `json:"name_id_format"`     // e.g., "urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress"
	AttributeMapping SAMLAttributeMapping `json:"attribute_mapping"`
	AllowIDPInitiated bool  `json:"allow_idp_initiated"`
}

// SAMLAttributeMapping maps SAML attributes to user fields
type SAMLAttributeMapping struct {
	Email     string `json:"email"`      // Attribute name for email
	FirstName string `json:"first_name"` // Attribute name for first name
	LastName  string `json:"last_name"`  // Attribute name for last name
	Groups    string `json:"groups"`     // Attribute name for groups (optional)
}

// DefaultSAMLAttributeMapping returns default attribute mappings
func DefaultSAMLAttributeMapping() SAMLAttributeMapping {
	return SAMLAttributeMapping{
		Email:     "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress",
		FirstName: "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/givenname",
		LastName:  "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/surname",
		Groups:    "http://schemas.xmlsoap.org/claims/Group",
	}
}

// SAMLManager handles SAML operations
type SAMLManager struct {
	configs      map[string]*SAMLConfig // orgID -> config
	pendingAuthn map[string]*samlAuthnRequest
	mu           sync.RWMutex
}

type samlAuthnRequest struct {
	ID        string
	OrgID     string
	IssueTime time.Time
	RelayState string
}

// SAMLUser represents user info extracted from SAML assertion
type SAMLUser struct {
	NameID    string
	Email     string
	FirstName string
	LastName  string
	Groups    []string
	Provider  string
}

// NewSAMLManager creates a new SAML manager
func NewSAMLManager() *SAMLManager {
	m := &SAMLManager{
		configs:      make(map[string]*SAMLConfig),
		pendingAuthn: make(map[string]*samlAuthnRequest),
	}

	// Start cleanup goroutine
	go m.cleanupPending()

	return m
}

// ConfigureOrg sets SAML configuration for an organization
func (m *SAMLManager) ConfigureOrg(orgID string, config *SAMLConfig) error {
	if config.SSOURL == "" || config.Certificate == "" {
		return ErrSAMLInvalidConfig
	}

	// Parse and validate certificate
	if _, err := m.parseCertificate(config.Certificate); err != nil {
		return fmt.Errorf("invalid certificate: %w", err)
	}

	m.mu.Lock()
	m.configs[orgID] = config
	m.mu.Unlock()

	return nil
}

// GetConfig returns SAML config for an organization
func (m *SAMLManager) GetConfig(orgID string) (*SAMLConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	config, ok := m.configs[orgID]
	if !ok || !config.Enabled {
		return nil, ErrSAMLNotConfigured
	}

	return config, nil
}

// RemoveConfig removes SAML configuration for an organization
func (m *SAMLManager) RemoveConfig(orgID string) {
	m.mu.Lock()
	delete(m.configs, orgID)
	m.mu.Unlock()
}

// CreateAuthnRequest creates a SAML AuthnRequest
func (m *SAMLManager) CreateAuthnRequest(orgID, acsURL, relayState string) (string, string, error) {
	config, err := m.GetConfig(orgID)
	if err != nil {
		return "", "", err
	}

	// Generate request ID
	requestID := "_" + generateSAMLID()
	issueInstant := time.Now().UTC().Format(time.RFC3339)

	// Build AuthnRequest XML
	authnRequest := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<samlp:AuthnRequest xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol"
    xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion"
    ID="%s"
    Version="2.0"
    IssueInstant="%s"
    Destination="%s"
    AssertionConsumerServiceURL="%s"
    ProtocolBinding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST">
    <saml:Issuer>%s</saml:Issuer>
    <samlp:NameIDPolicy Format="%s" AllowCreate="true"/>
</samlp:AuthnRequest>`,
		requestID,
		issueInstant,
		config.SSOURL,
		acsURL,
		config.EntityID,
		config.NameIDFormat,
	)

	// Store pending request
	m.mu.Lock()
	m.pendingAuthn[requestID] = &samlAuthnRequest{
		ID:         requestID,
		OrgID:      orgID,
		IssueTime:  time.Now(),
		RelayState: relayState,
	}
	m.mu.Unlock()

	// Deflate and base64 encode
	encoded, err := deflateAndEncode(authnRequest)
	if err != nil {
		return "", "", err
	}

	// Build redirect URL
	redirectURL := config.SSOURL + "?" + url.Values{
		"SAMLRequest": {encoded},
		"RelayState":  {relayState},
	}.Encode()

	return redirectURL, requestID, nil
}

// ProcessResponse processes a SAML response
func (m *SAMLManager) ProcessResponse(samlResponse, relayState string) (*SAMLUser, string, error) {
	// Decode response
	responseXML, err := base64.StdEncoding.DecodeString(samlResponse)
	if err != nil {
		return nil, "", fmt.Errorf("failed to decode response: %w", err)
	}

	// Parse response
	var response SAMLResponse
	if err := xml.Unmarshal(responseXML, &response); err != nil {
		return nil, "", fmt.Errorf("failed to parse response: %w", err)
	}

	// Validate InResponseTo if we have a pending request
	var orgID string
	if response.InResponseTo != "" {
		m.mu.Lock()
		pending, ok := m.pendingAuthn[response.InResponseTo]
		if ok {
			orgID = pending.OrgID
			delete(m.pendingAuthn, response.InResponseTo)
		}
		m.mu.Unlock()
	}

	// If no pending request, try to determine org from issuer or require IdP-initiated config
	if orgID == "" {
		orgID = m.findOrgByIssuer(response.Issuer.Value)
		if orgID == "" {
			return nil, "", ErrSAMLResponseInvalid
		}

		config, _ := m.GetConfig(orgID)
		if config != nil && !config.AllowIDPInitiated {
			return nil, "", errors.New("idp-initiated SSO not allowed")
		}
	}

	config, err := m.GetConfig(orgID)
	if err != nil {
		return nil, "", err
	}

	// Verify response status
	if response.Status.StatusCode.Value != "urn:oasis:names:tc:SAML:2.0:status:Success" {
		return nil, "", fmt.Errorf("SAML response status: %s", response.Status.StatusCode.Value)
	}

	// Get assertion (may be encrypted - skip encryption for now)
	if len(response.Assertion) == 0 {
		return nil, "", ErrSAMLResponseInvalid
	}

	assertion := response.Assertion[0]

	// Validate conditions
	if assertion.Conditions != nil {
		now := time.Now()
		if assertion.Conditions.NotBefore != "" {
			notBefore, _ := time.Parse(time.RFC3339, assertion.Conditions.NotBefore)
			if now.Before(notBefore.Add(-5 * time.Minute)) { // 5 min clock skew
				return nil, "", ErrSAMLAssertionExpired
			}
		}
		if assertion.Conditions.NotOnOrAfter != "" {
			notOnOrAfter, _ := time.Parse(time.RFC3339, assertion.Conditions.NotOnOrAfter)
			if now.After(notOnOrAfter.Add(5 * time.Minute)) { // 5 min clock skew
				return nil, "", ErrSAMLAssertionExpired
			}
		}
	}

	// Extract user info
	user := &SAMLUser{
		Provider: "saml",
	}

	// Get NameID
	if assertion.Subject != nil && assertion.Subject.NameID != nil {
		user.NameID = assertion.Subject.NameID.Value
	}

	// Extract attributes
	for _, stmt := range assertion.AttributeStatement {
		for _, attr := range stmt.Attributes {
			if len(attr.AttributeValue) == 0 {
				continue
			}
			value := attr.AttributeValue[0].Value

			switch attr.Name {
			case config.AttributeMapping.Email:
				user.Email = value
			case config.AttributeMapping.FirstName:
				user.FirstName = value
			case config.AttributeMapping.LastName:
				user.LastName = value
			case config.AttributeMapping.Groups:
				for _, av := range attr.AttributeValue {
					user.Groups = append(user.Groups, av.Value)
				}
			}
		}
	}

	// Fall back to NameID for email if not in attributes
	if user.Email == "" && strings.Contains(user.NameID, "@") {
		user.Email = user.NameID
	}

	if user.Email == "" {
		return nil, "", errors.New("no email found in SAML assertion")
	}

	return user, orgID, nil
}

// findOrgByIssuer finds organization by SAML issuer
func (m *SAMLManager) findOrgByIssuer(issuer string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for orgID, config := range m.configs {
		if config.Enabled && strings.Contains(config.SSOURL, issuer) {
			return orgID
		}
	}
	return ""
}

// parseCertificate parses a PEM-encoded certificate
func (m *SAMLManager) parseCertificate(certPEM string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		// Try decoding as raw base64
		certBytes, err := base64.StdEncoding.DecodeString(certPEM)
		if err != nil {
			return nil, errors.New("failed to decode certificate")
		}
		return x509.ParseCertificate(certBytes)
	}
	return x509.ParseCertificate(block.Bytes)
}

// cleanupPending cleans up expired pending requests
func (m *SAMLManager) cleanupPending() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		m.mu.Lock()
		now := time.Now()
		for id, req := range m.pendingAuthn {
			if now.Sub(req.IssueTime) > 10*time.Minute {
				delete(m.pendingAuthn, id)
			}
		}
		m.mu.Unlock()
	}
}

// GetMetadata returns SP metadata for an organization
func (m *SAMLManager) GetMetadata(orgID, acsURL, entityID string) (string, error) {
	metadata := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<md:EntityDescriptor xmlns:md="urn:oasis:names:tc:SAML:2.0:metadata"
    entityID="%s">
    <md:SPSSODescriptor AuthnRequestsSigned="false" WantAssertionsSigned="true"
        protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">
        <md:NameIDFormat>urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress</md:NameIDFormat>
        <md:AssertionConsumerService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST"
            Location="%s" index="0" isDefault="true"/>
    </md:SPSSODescriptor>
</md:EntityDescriptor>`,
		entityID,
		acsURL,
	)

	return metadata, nil
}

// SAML XML structures
type SAMLResponse struct {
	XMLName      xml.Name `xml:"Response"`
	ID           string   `xml:"ID,attr"`
	InResponseTo string   `xml:"InResponseTo,attr"`
	Issuer       SAMLIssuer `xml:"Issuer"`
	Status       SAMLStatus `xml:"Status"`
	Assertion    []SAMLAssertion `xml:"Assertion"`
}

type SAMLIssuer struct {
	Value string `xml:",chardata"`
}

type SAMLStatus struct {
	StatusCode SAMLStatusCode `xml:"StatusCode"`
}

type SAMLStatusCode struct {
	Value string `xml:"Value,attr"`
}

type SAMLAssertion struct {
	ID                 string `xml:"ID,attr"`
	Issuer             SAMLIssuer `xml:"Issuer"`
	Subject            *SAMLSubject `xml:"Subject"`
	Conditions         *SAMLConditions `xml:"Conditions"`
	AttributeStatement []SAMLAttributeStatement `xml:"AttributeStatement"`
}

type SAMLSubject struct {
	NameID *SAMLNameID `xml:"NameID"`
}

type SAMLNameID struct {
	Format string `xml:"Format,attr"`
	Value  string `xml:",chardata"`
}

type SAMLConditions struct {
	NotBefore    string `xml:"NotBefore,attr"`
	NotOnOrAfter string `xml:"NotOnOrAfter,attr"`
}

type SAMLAttributeStatement struct {
	Attributes []SAMLAttribute `xml:"Attribute"`
}

type SAMLAttribute struct {
	Name           string `xml:"Name,attr"`
	AttributeValue []SAMLAttributeValue `xml:"AttributeValue"`
}

type SAMLAttributeValue struct {
	Value string `xml:",chardata"`
}

// Helper functions

func generateSAMLID() string {
	b := make([]byte, 20)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func deflateAndEncode(data string) (string, error) {
	var buf strings.Builder
	w, err := flate.NewWriter(&buf, flate.BestCompression)
	if err != nil {
		return "", err
	}
	io.WriteString(w, data)
	w.Close()

	return base64.StdEncoding.EncodeToString([]byte(buf.String())), nil
}

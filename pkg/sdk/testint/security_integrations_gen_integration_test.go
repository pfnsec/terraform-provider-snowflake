package testint

import (
	"strings"
	"testing"

	"github.com/Snowflake-Labs/terraform-provider-snowflake/pkg/acceptance/helpers/random"
	"github.com/Snowflake-Labs/terraform-provider-snowflake/pkg/internal/snowflakeroles"
	"github.com/Snowflake-Labs/terraform-provider-snowflake/pkg/sdk"
	"github.com/Snowflake-Labs/terraform-provider-snowflake/pkg/sdk/internal/collections"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInt_SecurityIntegrations(t *testing.T) {
	client := testClient(t)
	ctx := testContext(t)

	acsURL := testClientHelper().Context.ACSURL(t)
	issuerURL := testClientHelper().Context.IssuerURL(t)
	cert := random.GenerateX509(t)
	rsaKey := random.GenerateRSAPublicKey(t)

	revertParameter := testClientHelper().Parameter.UpdateAccountParameterTemporarily(t, sdk.AccountParameterEnableIdentifierFirstLogin, "true")
	t.Cleanup(revertParameter)

	cleanupSecurityIntegration := func(t *testing.T, id sdk.AccountObjectIdentifier) {
		t.Helper()
		t.Cleanup(func() {
			err := client.SecurityIntegrations.Drop(ctx, sdk.NewDropSecurityIntegrationRequest(id).WithIfExists(true))
			assert.NoError(t, err)
		})
	}
	createOauthCustom := func(t *testing.T, with func(*sdk.CreateOauthForCustomClientsSecurityIntegrationRequest)) (*sdk.SecurityIntegration, sdk.AccountObjectIdentifier) {
		t.Helper()
		id := testClientHelper().Ids.RandomAccountObjectIdentifier()
		req := sdk.NewCreateOauthForCustomClientsSecurityIntegrationRequest(id, sdk.OauthSecurityIntegrationClientTypePublic, "https://example.com")
		if with != nil {
			with(req)
		}
		err := client.SecurityIntegrations.CreateOauthForCustomClients(ctx, req)
		require.NoError(t, err)
		cleanupSecurityIntegration(t, id)
		integration, err := client.SecurityIntegrations.ShowByID(ctx, id)
		require.NoError(t, err)

		return integration, id
	}
	createOauthPartner := func(t *testing.T, with func(*sdk.CreateOauthForPartnerApplicationsSecurityIntegrationRequest)) (*sdk.SecurityIntegration, sdk.AccountObjectIdentifier) {
		t.Helper()
		id := testClientHelper().Ids.RandomAccountObjectIdentifier()
		req := sdk.NewCreateOauthForPartnerApplicationsSecurityIntegrationRequest(id, sdk.OauthSecurityIntegrationClientLooker).
			WithOauthRedirectUri("http://example.com")

		if with != nil {
			with(req)
		}
		err := client.SecurityIntegrations.CreateOauthForPartnerApplications(ctx, req)
		require.NoError(t, err)
		cleanupSecurityIntegration(t, id)
		integration, err := client.SecurityIntegrations.ShowByID(ctx, id)
		require.NoError(t, err)

		return integration, id
	}
	createSAML2Integration := func(t *testing.T, with func(*sdk.CreateSaml2SecurityIntegrationRequest)) (*sdk.SecurityIntegration, sdk.AccountObjectIdentifier, string) {
		t.Helper()
		id := testClientHelper().Ids.RandomAccountObjectIdentifier()
		issuer := testClientHelper().Ids.Alpha()
		saml2Req := sdk.NewCreateSaml2SecurityIntegrationRequest(id, false, issuer, "https://example.com", "Custom", cert)
		if with != nil {
			with(saml2Req)
		}
		err := client.SecurityIntegrations.CreateSaml2(ctx, saml2Req)
		require.NoError(t, err)
		cleanupSecurityIntegration(t, id)
		integration, err := client.SecurityIntegrations.ShowByID(ctx, id)
		require.NoError(t, err)

		return integration, id, issuer
	}

	createSCIMIntegration := func(t *testing.T, with func(*sdk.CreateScimSecurityIntegrationRequest)) (*sdk.SecurityIntegration, sdk.AccountObjectIdentifier) {
		t.Helper()
		role, roleCleanup := testClientHelper().Role.CreateRoleWithRequest(t, sdk.NewCreateRoleRequest(snowflakeroles.GenericScimProvisioner).WithOrReplace(true))
		t.Cleanup(roleCleanup)
		testClientHelper().Role.GrantRoleToCurrentRole(t, role.ID())

		id := testClientHelper().Ids.RandomAccountObjectIdentifier()
		scimReq := sdk.NewCreateScimSecurityIntegrationRequest(id, false, sdk.ScimSecurityIntegrationScimClientGeneric, sdk.ScimSecurityIntegrationRunAsRoleGenericScimProvisioner)
		if with != nil {
			with(scimReq)
		}
		err := client.SecurityIntegrations.CreateScim(ctx, scimReq)
		require.NoError(t, err)
		cleanupSecurityIntegration(t, id)
		integration, err := client.SecurityIntegrations.ShowByID(ctx, id)
		require.NoError(t, err)

		return integration, id
	}

	assertSecurityIntegration := func(t *testing.T, si *sdk.SecurityIntegration, id sdk.AccountObjectIdentifier, siType string, enabled bool, comment string) {
		t.Helper()
		assert.Equal(t, id.Name(), si.Name)
		assert.Equal(t, siType, si.IntegrationType)
		assert.Equal(t, enabled, si.Enabled)
		assert.Equal(t, comment, si.Comment)
		assert.Equal(t, "SECURITY", si.Category)
	}

	type oauthPartnerDetails struct {
		enabled                 string
		oauthIssueRefreshTokens string
		refreshTokenValidity    string
		useSecondaryRoles       string
		preAuthorizedRolesList  string
		blockedRolesList        string
		networkPolicy           string
		comment                 string
	}

	assertOauthPartner := func(details []sdk.SecurityIntegrationProperty, d oauthPartnerDetails) {
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "ENABLED", Type: "Boolean", Value: d.enabled, Default: "false"})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "OAUTH_ISSUE_REFRESH_TOKENS", Type: "Boolean", Value: d.oauthIssueRefreshTokens, Default: "true"})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "OAUTH_REFRESH_TOKEN_VALIDITY", Type: "Integer", Value: d.refreshTokenValidity, Default: "7776000"})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "OAUTH_USE_SECONDARY_ROLES", Type: "String", Value: d.useSecondaryRoles, Default: "NONE"})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "PRE_AUTHORIZED_ROLES_LIST", Type: "List", Value: d.preAuthorizedRolesList, Default: "[]"})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "NETWORK_POLICY", Type: "String", Value: d.networkPolicy, Default: ""})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "COMMENT", Type: "String", Value: d.comment, Default: ""})
		// Check one-by-one because snowflake returns a few extra roles
		found, err := collections.FindOne(details, func(d sdk.SecurityIntegrationProperty) bool { return d.Name == "BLOCKED_ROLES_LIST" })
		assert.NoError(t, err)
		roles := strings.Split(found.Value, ",")
		for _, exp := range strings.Split(d.blockedRolesList, ",") {
			assert.Contains(t, roles, exp)
		}
	}

	assertOauthCustom := func(details []sdk.SecurityIntegrationProperty, d oauthPartnerDetails, allowNonTlsRedirectUri, clientType, enforcePkce string) {
		assertOauthPartner(details, d)
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "OAUTH_ALLOW_NON_TLS_REDIRECT_URI", Type: "Boolean", Value: allowNonTlsRedirectUri, Default: "false"})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "OAUTH_CLIENT_TYPE", Type: "String", Value: clientType, Default: "CONFIDENTIAL"})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "OAUTH_ENFORCE_PKCE", Type: "Boolean", Value: enforcePkce, Default: "false"})
		// Keys are hashed in snowflake, so we check only if these fields are present
		keys := make(map[string]struct{})
		for _, detail := range details {
			keys[detail.Name] = struct{}{}
		}
		assert.Contains(t, keys, "OAUTH_CLIENT_RSA_PUBLIC_KEY_FP")
		assert.Contains(t, keys, "OAUTH_CLIENT_RSA_PUBLIC_KEY_2_FP")
	}

	assertSCIMDescribe := func(details []sdk.SecurityIntegrationProperty, enabled, networkPolicy, runAsRole, syncPassword, comment string) {
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "ENABLED", Type: "Boolean", Value: enabled, Default: "false"})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "NETWORK_POLICY", Type: "String", Value: networkPolicy, Default: ""})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "RUN_AS_ROLE", Type: "String", Value: runAsRole, Default: ""})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "SYNC_PASSWORD", Type: "Boolean", Value: syncPassword, Default: "true"})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "COMMENT", Type: "String", Value: comment, Default: ""})
	}

	type saml2Details struct {
		provider                  string
		enableSPInitiated         string
		spInitiatedLoginPageLabel string
		ssoURL                    string
		issuer                    string
		requestedNameIDFormat     string
		forceAuthn                string
		postLogoutRedirectUrl     string
		signrequest               string
		comment                   string
		snowflakeIssuerURL        string
		snowflakeAcsURL           string
		allowedUserDomains        string
		allowedEmailPatterns      string
	}

	assertSAML2Describe := func(details []sdk.SecurityIntegrationProperty, d saml2Details) {
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "SAML2_X509_CERT", Type: "String", Value: cert, Default: ""})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "SAML2_PROVIDER", Type: "String", Value: d.provider, Default: ""})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "SAML2_ENABLE_SP_INITIATED", Type: "Boolean", Value: d.enableSPInitiated, Default: "false"})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "SAML2_SP_INITIATED_LOGIN_PAGE_LABEL", Type: "String", Value: d.spInitiatedLoginPageLabel, Default: ""})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "SAML2_SSO_URL", Type: "String", Value: d.ssoURL, Default: ""})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "SAML2_ISSUER", Type: "String", Value: d.issuer, Default: ""})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "SAML2_REQUESTED_NAMEID_FORMAT", Type: "String", Value: d.requestedNameIDFormat, Default: "urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress"})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "SAML2_FORCE_AUTHN", Type: "Boolean", Value: d.forceAuthn, Default: "false"})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "SAML2_POST_LOGOUT_REDIRECT_URL", Type: "String", Value: d.postLogoutRedirectUrl, Default: ""})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "SAML2_SIGN_REQUEST", Type: "Boolean", Value: d.signrequest, Default: "false"})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "SAML2_DIGEST_METHODS_USED", Type: "String", Value: "http://www.w3.org/2001/04/xmlenc#sha256", Default: ""})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "SAML2_SIGNATURE_METHODS_USED", Type: "String", Value: "http://www.w3.org/2001/04/xmldsig-more#rsa-sha256", Default: ""})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "COMMENT", Type: "String", Value: d.comment, Default: ""})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "SAML2_SNOWFLAKE_ISSUER_URL", Type: "String", Value: d.snowflakeIssuerURL, Default: ""})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "SAML2_SNOWFLAKE_ACS_URL", Type: "String", Value: d.snowflakeAcsURL, Default: ""})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "ALLOWED_USER_DOMAINS", Type: "List", Value: d.allowedUserDomains, Default: "[]"})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "ALLOWED_EMAIL_PATTERNS", Type: "List", Value: d.allowedEmailPatterns, Default: "[]"})
	}

	t.Run("CreateOauthPartner", func(t *testing.T) {
		role1, role1Cleanup := testClientHelper().Role.CreateRole(t)
		t.Cleanup(role1Cleanup)

		integration, id := createOauthPartner(t, func(r *sdk.CreateOauthForPartnerApplicationsSecurityIntegrationRequest) {
			r.WithBlockedRolesList(sdk.BlockedRolesListRequest{BlockedRolesList: []sdk.AccountObjectIdentifier{role1.ID()}}).
				WithComment("a").
				WithEnabled(true).
				WithOauthIssueRefreshTokens(true).
				WithOauthRefreshTokenValidity(12345).
				WithOauthUseSecondaryRoles(sdk.OauthSecurityIntegrationUseSecondaryRolesImplicit)
		})
		details, err := client.SecurityIntegrations.Describe(ctx, id)
		require.NoError(t, err)

		assertOauthPartner(details, oauthPartnerDetails{
			enabled:                 "true",
			oauthIssueRefreshTokens: "true",
			refreshTokenValidity:    "12345",
			useSecondaryRoles:       string(sdk.OauthSecurityIntegrationUseSecondaryRolesImplicit),
			blockedRolesList:        role1.Name,
			comment:                 "a",
		})

		assertSecurityIntegration(t, integration, id, "OAUTH - LOOKER", true, "a")
	})

	t.Run("CreateOauthCustom", func(t *testing.T) {
		networkPolicy, networkPolicyCleanup := testClientHelper().NetworkPolicy.CreateNetworkPolicy(t)
		t.Cleanup(networkPolicyCleanup)
		role1, role1Cleanup := testClientHelper().Role.CreateRole(t)
		t.Cleanup(role1Cleanup)
		role2, role2Cleanup := testClientHelper().Role.CreateRole(t)
		t.Cleanup(role2Cleanup)

		integration, id := createOauthCustom(t, func(r *sdk.CreateOauthForCustomClientsSecurityIntegrationRequest) {
			r.WithBlockedRolesList(sdk.BlockedRolesListRequest{BlockedRolesList: []sdk.AccountObjectIdentifier{role1.ID()}}).
				WithComment("a").
				WithEnabled(true).
				WithNetworkPolicy(sdk.NewAccountObjectIdentifier(networkPolicy.Name)).
				WithOauthAllowNonTlsRedirectUri(true).
				WithOauthClientRsaPublicKey(rsaKey).
				WithOauthClientRsaPublicKey2(rsaKey).
				WithOauthEnforcePkce(true).
				WithOauthIssueRefreshTokens(true).
				WithOauthRefreshTokenValidity(12345).
				WithOauthUseSecondaryRoles(sdk.OauthSecurityIntegrationUseSecondaryRolesImplicit).
				WithPreAuthorizedRolesList(sdk.PreAuthorizedRolesListRequest{PreAuthorizedRolesList: []sdk.AccountObjectIdentifier{role2.ID()}})
		})
		details, err := client.SecurityIntegrations.Describe(ctx, id)
		require.NoError(t, err)

		assertOauthCustom(details, oauthPartnerDetails{
			enabled:                 "true",
			oauthIssueRefreshTokens: "true",
			refreshTokenValidity:    "12345",
			useSecondaryRoles:       string(sdk.OauthSecurityIntegrationUseSecondaryRolesImplicit),
			preAuthorizedRolesList:  role2.Name,
			blockedRolesList:        role1.Name,
			networkPolicy:           networkPolicy.Name,
			comment:                 "a",
		}, "true", string(sdk.OauthSecurityIntegrationClientTypePublic), "true")

		assertSecurityIntegration(t, integration, id, "OAUTH - CUSTOM", true, "a")
	})

	t.Run("CreateSaml2", func(t *testing.T) {
		_, id, issuer := createSAML2Integration(t, func(r *sdk.CreateSaml2SecurityIntegrationRequest) {
			r.WithAllowedEmailPatterns([]sdk.EmailPattern{{Pattern: "^(.+dev)@example.com$"}}).
				WithAllowedUserDomains([]sdk.UserDomain{{Domain: "example.com"}}).
				WithComment("a").
				WithSaml2EnableSpInitiated(true).
				WithSaml2ForceAuthn(true).
				WithSaml2PostLogoutRedirectUrl("http://example.com/logout").
				WithSaml2RequestedNameidFormat("urn:oasis:names:tc:SAML:1.1:nameid-format:unspecified").
				WithSaml2SignRequest(true).
				WithSaml2SnowflakeAcsUrl(acsURL).
				WithSaml2SnowflakeIssuerUrl(issuerURL).
				WithSaml2SpInitiatedLoginPageLabel("label")
			// TODO: fix after format clarification
			// WithSaml2SnowflakeX509Cert(sdk.Pointer(x509))
		})
		details, err := client.SecurityIntegrations.Describe(ctx, id)
		require.NoError(t, err)

		assertSAML2Describe(details, saml2Details{
			provider:                  "Custom",
			enableSPInitiated:         "true",
			spInitiatedLoginPageLabel: "label",
			ssoURL:                    "https://example.com",
			issuer:                    issuer,
			requestedNameIDFormat:     "urn:oasis:names:tc:SAML:1.1:nameid-format:unspecified",
			forceAuthn:                "true",
			postLogoutRedirectUrl:     "http://example.com/logout",
			signrequest:               "true",
			comment:                   "a",
			snowflakeIssuerURL:        issuerURL,
			snowflakeAcsURL:           acsURL,
			allowedUserDomains:        "[example.com]",
			allowedEmailPatterns:      "[^(.+dev)@example.com$]",
		})

		si, err := client.SecurityIntegrations.ShowByID(ctx, id)
		require.NoError(t, err)
		assertSecurityIntegration(t, si, id, "SAML2", false, "a")
	})

	t.Run("CreateScim", func(t *testing.T) {
		networkPolicy, networkPolicyCleanup := testClientHelper().NetworkPolicy.CreateNetworkPolicy(t)
		t.Cleanup(networkPolicyCleanup)

		_, id := createSCIMIntegration(t, func(r *sdk.CreateScimSecurityIntegrationRequest) {
			r.WithComment("a").
				WithNetworkPolicy(sdk.NewAccountObjectIdentifier(networkPolicy.Name)).
				WithSyncPassword(false)
		})
		details, err := client.SecurityIntegrations.Describe(ctx, id)
		require.NoError(t, err)

		assertSCIMDescribe(details, "false", networkPolicy.Name, "GENERIC_SCIM_PROVISIONER", "false", "a")

		si, err := client.SecurityIntegrations.ShowByID(ctx, id)
		require.NoError(t, err)
		assertSecurityIntegration(t, si, id, "SCIM - GENERIC", false, "a")
	})

	t.Run("AlterOauthPartner", func(t *testing.T) {
		_, id := createOauthPartner(t, func(r *sdk.CreateOauthForPartnerApplicationsSecurityIntegrationRequest) {
			r.WithOauthRedirectUri("http://example.com")
		})
		role1, role1Cleanup := testClientHelper().Role.CreateRole(t)
		t.Cleanup(role1Cleanup)

		setRequest := sdk.NewAlterOauthForPartnerApplicationsSecurityIntegrationRequest(id).
			WithSet(
				*sdk.NewOauthForPartnerApplicationsIntegrationSetRequest().
					WithBlockedRolesList(*sdk.NewBlockedRolesListRequest().WithBlockedRolesList([]sdk.AccountObjectIdentifier{role1.ID()})).
					WithComment("a").
					WithEnabled(true).
					WithOauthIssueRefreshTokens(true).
					WithOauthRedirectUri("http://example2.com").
					WithOauthRefreshTokenValidity(22222).
					WithOauthUseSecondaryRoles(sdk.OauthSecurityIntegrationUseSecondaryRolesImplicit),
			)
		err := client.SecurityIntegrations.AlterOauthForPartnerApplications(ctx, setRequest)
		require.NoError(t, err)

		details, err := client.SecurityIntegrations.Describe(ctx, id)
		require.NoError(t, err)

		assertOauthPartner(details, oauthPartnerDetails{
			enabled:                 "true",
			oauthIssueRefreshTokens: "true",
			refreshTokenValidity:    "22222",
			useSecondaryRoles:       string(sdk.OauthSecurityIntegrationUseSecondaryRolesImplicit),
			preAuthorizedRolesList:  "",
			blockedRolesList:        "ACCOUNTADMIN,SECURITYADMIN",
			networkPolicy:           "",
			comment:                 "a",
		})

		unsetRequest := sdk.NewAlterOauthForPartnerApplicationsSecurityIntegrationRequest(id).
			WithUnset(
				*sdk.NewOauthForPartnerApplicationsIntegrationUnsetRequest().
					WithEnabled(true).
					WithOauthUseSecondaryRoles(true),
			)
		err = client.SecurityIntegrations.AlterOauthForPartnerApplications(ctx, unsetRequest)
		require.NoError(t, err)

		details, err = client.SecurityIntegrations.Describe(ctx, id)
		require.NoError(t, err)

		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "ENABLED", Type: "Boolean", Value: "false", Default: "false"})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "OAUTH_USE_SECONDARY_ROLES", Type: "String", Value: "NONE", Default: "NONE"})
	})

	t.Run("AlterOauthPartner - set and unset tags", func(t *testing.T) {
		tag, tagCleanup := testClientHelper().Tag.CreateTag(t)
		t.Cleanup(tagCleanup)

		_, id := createOauthPartner(t, nil)

		tagValue := "abc"
		tags := []sdk.TagAssociation{
			{
				Name:  tag.ID(),
				Value: tagValue,
			},
		}
		alterRequestSetTags := sdk.NewAlterOauthForPartnerApplicationsSecurityIntegrationRequest(id).WithSetTags(tags)

		err := client.SecurityIntegrations.AlterOauthForPartnerApplications(ctx, alterRequestSetTags)
		require.NoError(t, err)

		returnedTagValue, err := client.SystemFunctions.GetTag(ctx, tag.ID(), id, sdk.ObjectTypeIntegration)
		require.NoError(t, err)

		assert.Equal(t, tagValue, returnedTagValue)

		unsetTags := []sdk.ObjectIdentifier{
			tag.ID(),
		}
		alterRequestUnsetTags := sdk.NewAlterOauthForPartnerApplicationsSecurityIntegrationRequest(id).WithUnsetTags(unsetTags)

		err = client.SecurityIntegrations.AlterOauthForPartnerApplications(ctx, alterRequestUnsetTags)
		require.NoError(t, err)

		_, err = client.SystemFunctions.GetTag(ctx, tag.ID(), id, sdk.ObjectTypeIntegration)
		require.Error(t, err)
	})

	t.Run("AlterOauthCustom", func(t *testing.T) {
		_, id := createOauthCustom(t, nil)

		networkPolicy, networkPolicyCleanup := testClientHelper().NetworkPolicy.CreateNetworkPolicy(t)
		t.Cleanup(networkPolicyCleanup)
		role1, role1Cleanup := testClientHelper().Role.CreateRole(t)
		t.Cleanup(role1Cleanup)
		role2, role2Cleanup := testClientHelper().Role.CreateRole(t)
		t.Cleanup(role2Cleanup)

		setRequest := sdk.NewAlterOauthForCustomClientsSecurityIntegrationRequest(id).
			WithSet(
				*sdk.NewOauthForCustomClientsIntegrationSetRequest().
					WithEnabled(true).
					WithBlockedRolesList(sdk.BlockedRolesListRequest{BlockedRolesList: []sdk.AccountObjectIdentifier{role1.ID()}}).
					WithComment("a").
					WithNetworkPolicy(sdk.NewAccountObjectIdentifier(networkPolicy.Name)).
					WithOauthAllowNonTlsRedirectUri(true).
					WithOauthClientRsaPublicKey(rsaKey).
					WithOauthClientRsaPublicKey2(rsaKey).
					WithOauthEnforcePkce(true).
					WithOauthIssueRefreshTokens(true).
					WithOauthRedirectUri("http://example2.com").
					WithOauthRefreshTokenValidity(22222).
					WithOauthUseSecondaryRoles(sdk.OauthSecurityIntegrationUseSecondaryRolesImplicit).
					WithPreAuthorizedRolesList(sdk.PreAuthorizedRolesListRequest{PreAuthorizedRolesList: []sdk.AccountObjectIdentifier{role2.ID()}}),
			)
		err := client.SecurityIntegrations.AlterOauthForCustomClients(ctx, setRequest)
		require.NoError(t, err)

		details, err := client.SecurityIntegrations.Describe(ctx, id)
		require.NoError(t, err)

		assertOauthCustom(details, oauthPartnerDetails{
			enabled:                 "true",
			oauthIssueRefreshTokens: "true",
			refreshTokenValidity:    "22222",
			useSecondaryRoles:       string(sdk.OauthSecurityIntegrationUseSecondaryRolesImplicit),
			preAuthorizedRolesList:  role2.Name,
			blockedRolesList:        role1.Name,
			networkPolicy:           networkPolicy.Name,
			comment:                 "a",
		}, "true", string(sdk.OauthSecurityIntegrationClientTypePublic), "true")

		unsetRequest := sdk.NewAlterOauthForCustomClientsSecurityIntegrationRequest(id).
			WithUnset(
				*sdk.NewOauthForCustomClientsIntegrationUnsetRequest().
					WithEnabled(true).
					WithOauthUseSecondaryRoles(true).
					WithNetworkPolicy(true).
					WithOauthClientRsaPublicKey(true).
					WithOauthClientRsaPublicKey2(true),
			)
		err = client.SecurityIntegrations.AlterOauthForCustomClients(ctx, unsetRequest)
		require.NoError(t, err)

		details, err = client.SecurityIntegrations.Describe(ctx, id)
		require.NoError(t, err)

		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "ENABLED", Type: "Boolean", Value: "false", Default: "false"})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "OAUTH_USE_SECONDARY_ROLES", Type: "String", Value: "NONE", Default: "NONE"})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "NETWORK_POLICY", Type: "String", Value: "", Default: ""})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "OAUTH_CLIENT_RSA_PUBLIC_KEY_FP", Type: "String", Value: "", Default: ""})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "OAUTH_CLIENT_RSA_PUBLIC_KEY_2_FP", Type: "String", Value: "", Default: ""})
	})

	t.Run("AlterOauthCustom - set and unset tags", func(t *testing.T) {
		tag, tagCleanup := testClientHelper().Tag.CreateTag(t)
		t.Cleanup(tagCleanup)

		_, id := createOauthCustom(t, nil)

		tagValue := "abc"
		tags := []sdk.TagAssociation{
			{
				Name:  tag.ID(),
				Value: tagValue,
			},
		}
		alterRequestSetTags := sdk.NewAlterOauthForCustomClientsSecurityIntegrationRequest(id).WithSetTags(tags)

		err := client.SecurityIntegrations.AlterOauthForCustomClients(ctx, alterRequestSetTags)
		require.NoError(t, err)

		returnedTagValue, err := client.SystemFunctions.GetTag(ctx, tag.ID(), id, sdk.ObjectTypeIntegration)
		require.NoError(t, err)

		assert.Equal(t, tagValue, returnedTagValue)

		unsetTags := []sdk.ObjectIdentifier{
			tag.ID(),
		}
		alterRequestUnsetTags := sdk.NewAlterOauthForCustomClientsSecurityIntegrationRequest(id).WithUnsetTags(unsetTags)

		err = client.SecurityIntegrations.AlterOauthForCustomClients(ctx, alterRequestUnsetTags)
		require.NoError(t, err)

		_, err = client.SystemFunctions.GetTag(ctx, tag.ID(), id, sdk.ObjectTypeIntegration)
		require.Error(t, err)
	})
	t.Run("AlterSAML2Integration", func(t *testing.T) {
		_, id, issuer := createSAML2Integration(t, nil)

		setRequest := sdk.NewAlterSaml2SecurityIntegrationRequest(id).
			WithSet(
				*sdk.NewSaml2IntegrationSetRequest().
					WithEnabled(true).
					WithSaml2Issuer(issuer).
					WithSaml2SsoUrl("http://example.com").
					WithSaml2Provider("OKTA").
					WithSaml2X509Cert(cert).
					WithComment("a").
					WithSaml2EnableSpInitiated(true).
					WithSaml2ForceAuthn(true).
					WithSaml2PostLogoutRedirectUrl("http://example.com/logout").
					WithSaml2RequestedNameidFormat("urn:oasis:names:tc:SAML:1.1:nameid-format:unspecified").
					WithSaml2SignRequest(true).
					WithSaml2SnowflakeAcsUrl(acsURL).
					WithSaml2SnowflakeIssuerUrl(issuerURL).
					WithSaml2SpInitiatedLoginPageLabel("label").
					WithAllowedEmailPatterns([]sdk.EmailPattern{{Pattern: "^(.+dev)@example.com$"}}).
					WithAllowedUserDomains([]sdk.UserDomain{{Domain: "example.com"}}),
				// TODO: fix after format clarification
				// WithSaml2SnowflakeX509Cert(sdk.Pointer(cert)).
			)
		err := client.SecurityIntegrations.AlterSaml2(ctx, setRequest)
		require.NoError(t, err)

		details, err := client.SecurityIntegrations.Describe(ctx, id)
		require.NoError(t, err)

		assertSAML2Describe(details, saml2Details{
			provider:                  "OKTA",
			enableSPInitiated:         "true",
			spInitiatedLoginPageLabel: "label",
			ssoURL:                    "http://example.com",
			issuer:                    issuer,
			requestedNameIDFormat:     "urn:oasis:names:tc:SAML:1.1:nameid-format:unspecified",
			forceAuthn:                "true",
			postLogoutRedirectUrl:     "http://example.com/logout",
			signrequest:               "true",
			comment:                   "a",
			snowflakeIssuerURL:        issuerURL,
			snowflakeAcsURL:           acsURL,
			allowedUserDomains:        "[example.com]",
			allowedEmailPatterns:      "[^(.+dev)@example.com$]",
		})

		unsetRequest := sdk.NewAlterSaml2SecurityIntegrationRequest(id).
			WithUnset(
				*sdk.NewSaml2IntegrationUnsetRequest().
					WithSaml2ForceAuthn(true).
					WithSaml2RequestedNameidFormat(true).
					WithSaml2PostLogoutRedirectUrl(true).
					WithComment(true),
			)
		err = client.SecurityIntegrations.AlterSaml2(ctx, unsetRequest)
		require.NoError(t, err)

		details, err = client.SecurityIntegrations.Describe(ctx, id)
		require.NoError(t, err)
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "SAML2_FORCE_AUTHN", Type: "Boolean", Value: "false", Default: "false"})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "SAML2_REQUESTED_NAMEID_FORMAT", Type: "String", Value: "urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress", Default: "urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress"})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "SAML2_POST_LOGOUT_REDIRECT_URL", Type: "String", Value: "", Default: ""})
		assert.Contains(t, details, sdk.SecurityIntegrationProperty{Name: "COMMENT", Type: "String", Value: "", Default: ""})
	})

	t.Run("AlterSAML2Integration - REFRESH SAML2_SNOWFLAKE_PRIVATE_KEY", func(t *testing.T) {
		_, id, _ := createSAML2Integration(t, nil)

		setRequest := sdk.NewAlterSaml2SecurityIntegrationRequest(id).WithRefreshSaml2SnowflakePrivateKey(true)
		err := client.SecurityIntegrations.AlterSaml2(ctx, setRequest)
		require.NoError(t, err)
	})

	t.Run("AlterSAML2Integration - set and unset tags", func(t *testing.T) {
		tag, tagCleanup := testClientHelper().Tag.CreateTag(t)
		t.Cleanup(tagCleanup)

		_, id, _ := createSAML2Integration(t, nil)

		tagValue := "abc"
		tags := []sdk.TagAssociation{
			{
				Name:  tag.ID(),
				Value: tagValue,
			},
		}
		alterRequestSetTags := sdk.NewAlterSaml2SecurityIntegrationRequest(id).WithSetTags(tags)

		err := client.SecurityIntegrations.AlterSaml2(ctx, alterRequestSetTags)
		require.NoError(t, err)

		returnedTagValue, err := client.SystemFunctions.GetTag(ctx, tag.ID(), id, sdk.ObjectTypeIntegration)
		require.NoError(t, err)

		assert.Equal(t, tagValue, returnedTagValue)

		unsetTags := []sdk.ObjectIdentifier{
			tag.ID(),
		}
		alterRequestUnsetTags := sdk.NewAlterSaml2SecurityIntegrationRequest(id).WithUnsetTags(unsetTags)

		err = client.SecurityIntegrations.AlterSaml2(ctx, alterRequestUnsetTags)
		require.NoError(t, err)

		_, err = client.SystemFunctions.GetTag(ctx, tag.ID(), id, sdk.ObjectTypeIntegration)
		require.Error(t, err)
	})

	t.Run("AlterSCIMIntegration", func(t *testing.T) {
		_, id := createSCIMIntegration(t, nil)

		networkPolicy, networkPolicyCleanup := testClientHelper().NetworkPolicy.CreateNetworkPolicy(t)
		t.Cleanup(networkPolicyCleanup)

		setRequest := sdk.NewAlterScimSecurityIntegrationRequest(id).
			WithSet(
				*sdk.NewScimIntegrationSetRequest().
					WithNetworkPolicy(sdk.NewAccountObjectIdentifier(networkPolicy.Name)).
					WithEnabled(true).
					WithSyncPassword(false).
					WithComment("altered"),
			)
		err := client.SecurityIntegrations.AlterScim(ctx, setRequest)
		require.NoError(t, err)

		details, err := client.SecurityIntegrations.Describe(ctx, id)
		require.NoError(t, err)

		assertSCIMDescribe(details, "true", networkPolicy.Name, "GENERIC_SCIM_PROVISIONER", "false", "altered")

		unsetRequest := sdk.NewAlterScimSecurityIntegrationRequest(id).
			WithUnset(
				*sdk.NewScimIntegrationUnsetRequest().
					WithNetworkPolicy(true).
					WithSyncPassword(true),
			)
		err = client.SecurityIntegrations.AlterScim(ctx, unsetRequest)
		require.NoError(t, err)

		details, err = client.SecurityIntegrations.Describe(ctx, id)
		require.NoError(t, err)

		assertSCIMDescribe(details, "true", "", "GENERIC_SCIM_PROVISIONER", "true", "altered")
	})

	t.Run("AlterSCIMIntegration - set and unset tags", func(t *testing.T) {
		tag, tagCleanup := testClientHelper().Tag.CreateTag(t)
		t.Cleanup(tagCleanup)

		_, id := createSCIMIntegration(t, nil)

		tagValue := "abc"
		tags := []sdk.TagAssociation{
			{
				Name:  tag.ID(),
				Value: tagValue,
			},
		}
		alterRequestSetTags := sdk.NewAlterScimSecurityIntegrationRequest(id).WithSetTags(tags)

		err := client.SecurityIntegrations.AlterScim(ctx, alterRequestSetTags)
		require.NoError(t, err)

		returnedTagValue, err := client.SystemFunctions.GetTag(ctx, tag.ID(), id, sdk.ObjectTypeIntegration)
		require.NoError(t, err)

		assert.Equal(t, tagValue, returnedTagValue)

		unsetTags := []sdk.ObjectIdentifier{
			tag.ID(),
		}
		alterRequestUnsetTags := sdk.NewAlterScimSecurityIntegrationRequest(id).WithUnsetTags(unsetTags)

		err = client.SecurityIntegrations.AlterScim(ctx, alterRequestUnsetTags)
		require.NoError(t, err)

		_, err = client.SystemFunctions.GetTag(ctx, tag.ID(), id, sdk.ObjectTypeIntegration)
		require.Error(t, err)
	})

	t.Run("Drop", func(t *testing.T) {
		_, id := createSCIMIntegration(t, nil)

		si, err := client.SecurityIntegrations.ShowByID(ctx, id)
		require.NotNil(t, si)
		require.NoError(t, err)

		err = client.SecurityIntegrations.Drop(ctx, sdk.NewDropSecurityIntegrationRequest(id))
		require.NoError(t, err)

		si, err = client.SecurityIntegrations.ShowByID(ctx, id)
		require.Nil(t, si)
		require.Error(t, err)
	})

	t.Run("Drop non-existing", func(t *testing.T) {
		id := sdk.NewAccountObjectIdentifier("does_not_exist")

		err := client.SecurityIntegrations.Drop(ctx, sdk.NewDropSecurityIntegrationRequest(id))
		assert.ErrorIs(t, err, sdk.ErrObjectNotExistOrAuthorized)
	})

	t.Run("Describe", func(t *testing.T) {
		_, id := createSCIMIntegration(t, nil)

		details, err := client.SecurityIntegrations.Describe(ctx, id)
		require.NoError(t, err)

		assertSCIMDescribe(details, "false", "", "GENERIC_SCIM_PROVISIONER", "true", "")
	})

	t.Run("ShowByID", func(t *testing.T) {
		_, id := createSCIMIntegration(t, nil)

		si, err := client.SecurityIntegrations.ShowByID(ctx, id)
		require.NoError(t, err)
		assertSecurityIntegration(t, si, id, "SCIM - GENERIC", false, "")
	})

	t.Run("Show OauthPartner", func(t *testing.T) {
		si1, id1 := createOauthPartner(t, nil)
		// more than one oauth partner integration is not allowed, create a custom one
		si2, _ := createOauthCustom(t, nil)

		returnedIntegrations, err := client.SecurityIntegrations.Show(ctx, sdk.NewShowSecurityIntegrationRequest().WithLike(sdk.Like{
			Pattern: sdk.Pointer(id1.Name()),
		}))
		require.NoError(t, err)
		assert.Contains(t, returnedIntegrations, *si1)
		assert.NotContains(t, returnedIntegrations, *si2)
	})

	t.Run("Show OauthCustom", func(t *testing.T) {
		si1, id1 := createOauthCustom(t, nil)
		si2, _ := createOauthCustom(t, nil)

		returnedIntegrations, err := client.SecurityIntegrations.Show(ctx, sdk.NewShowSecurityIntegrationRequest().WithLike(sdk.Like{
			Pattern: sdk.Pointer(id1.Name()),
		}))
		require.NoError(t, err)
		assert.Contains(t, returnedIntegrations, *si1)
		assert.NotContains(t, returnedIntegrations, *si2)
	})

	t.Run("Show SAML2", func(t *testing.T) {
		si1, id1, _ := createSAML2Integration(t, nil)
		si2, _, _ := createSAML2Integration(t, nil)

		returnedIntegrations, err := client.SecurityIntegrations.Show(ctx, sdk.NewShowSecurityIntegrationRequest().WithLike(sdk.Like{
			Pattern: sdk.Pointer(id1.Name()),
		}))
		require.NoError(t, err)
		assert.Contains(t, returnedIntegrations, *si1)
		assert.NotContains(t, returnedIntegrations, *si2)
	})

	t.Run("Show SCIM", func(t *testing.T) {
		si1, id1 := createSCIMIntegration(t, nil)
		si2, _ := createSCIMIntegration(t, nil)

		returnedIntegrations, err := client.SecurityIntegrations.Show(ctx, sdk.NewShowSecurityIntegrationRequest().WithLike(sdk.Like{
			Pattern: sdk.Pointer(id1.Name()),
		}))
		require.NoError(t, err)
		assert.Contains(t, returnedIntegrations, *si1)
		assert.NotContains(t, returnedIntegrations, *si2)
	})
}

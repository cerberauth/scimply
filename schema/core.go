package schema

const (
	UserSchemaURI           = "urn:ietf:params:scim:schemas:core:2.0:User"
	GroupSchemaURI          = "urn:ietf:params:scim:schemas:core:2.0:Group"
	EnterpriseUserSchemaURI = "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"

	valueAttr       = "value"
	displayAttr     = "display"
	typeAttr        = "type"
	primaryAttr     = "primary"
	userNameAttr    = "userName"
	displayNameAttr = "displayName"
	emailsAttr      = "emails"
	activeAttr      = "active"
	formattedAttr   = "formatted"
	groupRef        = "Group"
	refAttr         = "$ref"
	idAttr          = "id"
	externalIDAttr  = "externalId"

	descDisplay       = "A human-readable name, primarily used for display purposes."
	descType          = "A label indicating the attribute's function."
	descPrimary       = "A Boolean value indicating the 'primary' or preferred attribute value."
	descUserResource  = "User"
	descGroupResource = "Group"

	workAttr           = "work"
	homeAttr           = "home"
	otherAttr          = "other"
	employeeNumberAttr = "employeeNumber"
	usersEndpoint      = "/Users"
	groupsEndpoint     = "/Groups"
)

func commonSubAttrs(typeCanonicalValues []string) []Attribute {
	return []Attribute{
		{
			Name:        valueAttr,
			Type:        TypeString,
			Description: "The attribute's significant value.",
			Mutability:  MutabilityReadWrite,
			Returned:    ReturnedDefault,
			Uniqueness:  UniquenessNone,
		},
		{
			Name:        displayAttr,
			Type:        TypeString,
			Description: descDisplay,
			Mutability:  MutabilityReadWrite,
			Returned:    ReturnedDefault,
			Uniqueness:  UniquenessNone,
		},
		{
			Name:            typeAttr,
			Type:            TypeString,
			Description:     descType,
			CanonicalValues: typeCanonicalValues,
			Mutability:      MutabilityReadWrite,
			Returned:        ReturnedDefault,
			Uniqueness:      UniquenessNone,
		},
		{
			Name:        primaryAttr,
			Type:        TypeBoolean,
			Description: descPrimary,
			Mutability:  MutabilityReadWrite,
			Returned:    ReturnedDefault,
			Uniqueness:  UniquenessNone,
		},
	}
}

func CoreUserSchema() *Schema {
	return &Schema{
		ID:          UserSchemaURI,
		Name:        descUserResource,
		Description: "User Account",
		Attributes: []Attribute{
			{
				Name:        idAttr,
				Type:        TypeString,
				Description: "A unique identifier for a SCIM resource as defined by the service provider.",
				Required:    false,
				CaseExact:   true,
				Mutability:  MutabilityReadOnly,
				Returned:    ReturnedAlways,
				Uniqueness:  UniquenessServer,
			},
			{
				Name:        externalIDAttr,
				Type:        TypeString,
				Description: "A String that is an identifier for the resource as defined by the provisioning client.",
				Mutability:  MutabilityReadWrite,
				Returned:    ReturnedDefault,
				Uniqueness:  UniquenessNone,
			},
			{
				Name:        userNameAttr,
				Type:        TypeString,
				Description: "Unique identifier for the User, typically used by the user to directly authenticate to the service provider.",
				Required:    true,
				CaseExact:   false,
				Mutability:  MutabilityReadWrite,
				Returned:    ReturnedDefault,
				Uniqueness:  UniquenessServer,
			},
			{
				Name:        "name",
				Type:        TypeComplex,
				Description: "The components of the user's real name.",
				Mutability:  MutabilityReadWrite,
				Returned:    ReturnedDefault,
				Uniqueness:  UniquenessNone,
				SubAttributes: []Attribute{
					{
						Name:        formattedAttr,
						Type:        TypeString,
						Description: "The full name, including all middle names, titles, and suffixes as appropriate, formatted for display.",
						Mutability:  MutabilityReadWrite,
						Returned:    ReturnedDefault,
						Uniqueness:  UniquenessNone,
					},
					{
						Name:        "familyName",
						Type:        TypeString,
						Description: "The family name of the User, or last name in most Western languages.",
						Mutability:  MutabilityReadWrite,
						Returned:    ReturnedDefault,
						Uniqueness:  UniquenessNone,
					},
					{
						Name:        "givenName",
						Type:        TypeString,
						Description: "The given name of the User, or first name in most Western languages.",
						Mutability:  MutabilityReadWrite,
						Returned:    ReturnedDefault,
						Uniqueness:  UniquenessNone,
					},
					{
						Name:        "middleName",
						Type:        TypeString,
						Description: "The middle name(s) of the User.",
						Mutability:  MutabilityReadWrite,
						Returned:    ReturnedDefault,
						Uniqueness:  UniquenessNone,
					},
					{
						Name:        "honorificPrefix",
						Type:        TypeString,
						Description: "The honorific prefix(es) of the User, or title in most Western languages.",
						Mutability:  MutabilityReadWrite,
						Returned:    ReturnedDefault,
						Uniqueness:  UniquenessNone,
					},
					{
						Name:        "honorificSuffix",
						Type:        TypeString,
						Description: "The honorific suffix(es) of the User, or suffix in most Western languages.",
						Mutability:  MutabilityReadWrite,
						Returned:    ReturnedDefault,
						Uniqueness:  UniquenessNone,
					},
				},
			},
			{
				Name:        displayNameAttr,
				Type:        TypeString,
				Description: "The name of the User, suitable for display to end-users.",
				Mutability:  MutabilityReadWrite,
				Returned:    ReturnedDefault,
				Uniqueness:  UniquenessNone,
			},
			{
				Name:        "nickName",
				Type:        TypeString,
				Description: "The casual way to address the user in real life.",
				Mutability:  MutabilityReadWrite,
				Returned:    ReturnedDefault,
				Uniqueness:  UniquenessNone,
			},
			{
				Name:           "profileUrl",
				Type:           TypeReference,
				Description:    "A fully qualified URL pointing to a page representing the User's online profile.",
				Mutability:     MutabilityReadWrite,
				Returned:       ReturnedDefault,
				Uniqueness:     UniquenessNone,
				ReferenceTypes: []string{"external"},
			},
			{
				Name:        "title",
				Type:        TypeString,
				Description: "The user's title, such as \"Vice President\".",
				Mutability:  MutabilityReadWrite,
				Returned:    ReturnedDefault,
				Uniqueness:  UniquenessNone,
			},
			{
				Name:        "userType",
				Type:        TypeString,
				Description: "Used to identify the relationship between the organization and the user.",
				Mutability:  MutabilityReadWrite,
				Returned:    ReturnedDefault,
				Uniqueness:  UniquenessNone,
			},
			{
				Name:        "preferredLanguage",
				Type:        TypeString,
				Description: "Indicates the User's preferred written or spoken language.",
				Mutability:  MutabilityReadWrite,
				Returned:    ReturnedDefault,
				Uniqueness:  UniquenessNone,
			},
			{
				Name:        "locale",
				Type:        TypeString,
				Description: "Used to indicate the User's default location for purposes of localizing items such as currency, date time format, or numerical representations.",
				Mutability:  MutabilityReadWrite,
				Returned:    ReturnedDefault,
				Uniqueness:  UniquenessNone,
			},
			{
				Name:        "timezone",
				Type:        TypeString,
				Description: "The User's time zone in the 'Olson' time zone database format.",
				Mutability:  MutabilityReadWrite,
				Returned:    ReturnedDefault,
				Uniqueness:  UniquenessNone,
			},
			{
				Name:        activeAttr,
				Type:        TypeBoolean,
				Description: "A Boolean value indicating the User's administrative status.",
				Mutability:  MutabilityReadWrite,
				Returned:    ReturnedDefault,
				Uniqueness:  UniquenessNone,
			},
			{
				Name:        "password",
				Type:        TypeString,
				Description: "The User's cleartext password. This attribute is intended to be used as a means to specify an initial password when creating a new User or to reset an existing User's password.",
				Mutability:  MutabilityWriteOnly,
				Returned:    ReturnedNever,
				Uniqueness:  UniquenessNone,
				CaseExact:   true,
			},

			{
				Name:          emailsAttr,
				Type:          TypeComplex,
				MultiValued:   true,
				Description:   "Email addresses for the user.",
				Mutability:    MutabilityReadWrite,
				Returned:      ReturnedDefault,
				Uniqueness:    UniquenessNone,
				SubAttributes: commonSubAttrs([]string{workAttr, homeAttr, otherAttr}),
			},

			{
				Name:          "phoneNumbers",
				Type:          TypeComplex,
				MultiValued:   true,
				Description:   "Phone numbers for the User.",
				Mutability:    MutabilityReadWrite,
				Returned:      ReturnedDefault,
				Uniqueness:    UniquenessNone,
				SubAttributes: commonSubAttrs([]string{workAttr, homeAttr, "mobile", "fax", "pager", otherAttr}),
			},

			{
				Name:          "ims",
				Type:          TypeComplex,
				MultiValued:   true,
				Description:   "Instant messaging addresses for the User.",
				Mutability:    MutabilityReadWrite,
				Returned:      ReturnedDefault,
				Uniqueness:    UniquenessNone,
				SubAttributes: commonSubAttrs([]string{"aim", "gtalk", "icq", "xmpp", "msn", "skype", "qq", "yahoo"}),
			},

			{
				Name:        "photos",
				Type:        TypeComplex,
				MultiValued: true,
				Description: "URLs of photos of the User.",
				Mutability:  MutabilityReadWrite,
				Returned:    ReturnedDefault,
				Uniqueness:  UniquenessNone,
				SubAttributes: []Attribute{
					{
						Name:           valueAttr,
						Type:           TypeReference,
						Description:    "URL of a photo of the User.",
						Mutability:     MutabilityReadWrite,
						Returned:       ReturnedDefault,
						Uniqueness:     UniquenessNone,
						ReferenceTypes: []string{"external"},
					},
					{
						Name:        displayAttr,
						Type:        TypeString,
						Description: descDisplay,
						Mutability:  MutabilityReadWrite,
						Returned:    ReturnedDefault,
						Uniqueness:  UniquenessNone,
					},
					{
						Name:            typeAttr,
						Type:            TypeString,
						Description:     descType,
						CanonicalValues: []string{"photo", "thumbnail"},
						Mutability:      MutabilityReadWrite,
						Returned:        ReturnedDefault,
						Uniqueness:      UniquenessNone,
					},
					{
						Name:        primaryAttr,
						Type:        TypeBoolean,
						Description: descPrimary,
						Mutability:  MutabilityReadWrite,
						Returned:    ReturnedDefault,
						Uniqueness:  UniquenessNone,
					},
				},
			},

			{
				Name:        "addresses",
				Type:        TypeComplex,
				MultiValued: true,
				Description: "A physical mailing address for this User.",
				Mutability:  MutabilityReadWrite,
				Returned:    ReturnedDefault,
				Uniqueness:  UniquenessNone,
				SubAttributes: []Attribute{
					{
						Name:        "formatted",
						Type:        TypeString,
						Description: "The full mailing address, formatted for display or use with a mailing label.",
						Mutability:  MutabilityReadWrite,
						Returned:    ReturnedDefault,
						Uniqueness:  UniquenessNone,
					},
					{
						Name:        "streetAddress",
						Type:        TypeString,
						Description: "The full street address component, which may include house number, street name, P.O. box, and multi-line extended street address information.",
						Mutability:  MutabilityReadWrite,
						Returned:    ReturnedDefault,
						Uniqueness:  UniquenessNone,
					},
					{
						Name:        "locality",
						Type:        TypeString,
						Description: "The city or locality component.",
						Mutability:  MutabilityReadWrite,
						Returned:    ReturnedDefault,
						Uniqueness:  UniquenessNone,
					},
					{
						Name:        "region",
						Type:        TypeString,
						Description: "The state or region component.",
						Mutability:  MutabilityReadWrite,
						Returned:    ReturnedDefault,
						Uniqueness:  UniquenessNone,
					},
					{
						Name:        "postalCode",
						Type:        TypeString,
						Description: "The zip code or postal code component.",
						Mutability:  MutabilityReadWrite,
						Returned:    ReturnedDefault,
						Uniqueness:  UniquenessNone,
					},
					{
						Name:        "country",
						Type:        TypeString,
						Description: "The country name component.",
						Mutability:  MutabilityReadWrite,
						Returned:    ReturnedDefault,
						Uniqueness:  UniquenessNone,
					},
					{
						Name:            typeAttr,
						Type:            TypeString,
						Description:     descType,
						CanonicalValues: []string{workAttr, homeAttr, otherAttr},
						Mutability:      MutabilityReadWrite,
						Returned:        ReturnedDefault,
						Uniqueness:      UniquenessNone,
					},
					{
						Name:        primaryAttr,
						Type:        TypeBoolean,
						Description: descPrimary,
						Mutability:  MutabilityReadWrite,
						Returned:    ReturnedDefault,
						Uniqueness:  UniquenessNone,
					},
				},
			},

			{
				Name:        "groups",
				Type:        TypeComplex,
				MultiValued: true,
				Description: "A list of groups to which the user belongs.",
				Mutability:  MutabilityReadOnly,
				Returned:    ReturnedDefault,
				Uniqueness:  UniquenessNone,
				SubAttributes: []Attribute{
					{
						Name:           valueAttr,
						Type:           TypeReference,
						Description:    "The identifier of the User's group.",
						Mutability:     MutabilityReadOnly,
						Returned:       ReturnedDefault,
						Uniqueness:     UniquenessNone,
						ReferenceTypes: []string{groupRef},
					},
					{
						Name:           refAttr,
						Type:           TypeReference,
						Description:    "The URI of the corresponding Group resource to which the user belongs.",
						Mutability:     MutabilityReadOnly,
						Returned:       ReturnedDefault,
						Uniqueness:     UniquenessNone,
						ReferenceTypes: []string{groupRef},
					},
					{
						Name:        displayAttr,
						Type:        TypeString,
						Description: descDisplay,
						Mutability:  MutabilityReadOnly,
						Returned:    ReturnedDefault,
						Uniqueness:  UniquenessNone,
					},
					{
						Name:            "type",
						Type:            TypeString,
						Description:     "A label indicating the attribute's function, e.g., 'direct' or 'indirect'.",
						CanonicalValues: []string{"direct", "indirect"},
						Mutability:      MutabilityReadOnly,
						Returned:        ReturnedDefault,
						Uniqueness:      UniquenessNone,
					},
				},
			},

			{
				Name:          "entitlements",
				Type:          TypeComplex,
				MultiValued:   true,
				Description:   "A list of entitlements for the User that represent a thing the User has.",
				Mutability:    MutabilityReadWrite,
				Returned:      ReturnedDefault,
				Uniqueness:    UniquenessNone,
				SubAttributes: commonSubAttrs(nil),
			},

			{
				Name:          "roles",
				Type:          TypeComplex,
				MultiValued:   true,
				Description:   "A list of roles for the User that collectively represent who the User is.",
				Mutability:    MutabilityReadWrite,
				Returned:      ReturnedDefault,
				Uniqueness:    UniquenessNone,
				SubAttributes: commonSubAttrs(nil),
			},

			{
				Name:        "x509Certificates",
				Type:        TypeComplex,
				MultiValued: true,
				Description: "A list of certificates issued to the User.",
				Mutability:  MutabilityReadWrite,
				Returned:    ReturnedDefault,
				Uniqueness:  UniquenessNone,
				SubAttributes: []Attribute{
					{
						Name:        valueAttr,
						Type:        TypeBinary,
						Description: "The value of an X.509 certificate.",
						Mutability:  MutabilityReadWrite,
						Returned:    ReturnedDefault,
						Uniqueness:  UniquenessNone,
					},
					{
						Name:        displayAttr,
						Type:        TypeString,
						Description: descDisplay,
						Mutability:  MutabilityReadWrite,
						Returned:    ReturnedDefault,
						Uniqueness:  UniquenessNone,
					},
					{
						Name:        typeAttr,
						Type:        TypeString,
						Description: descType,
						Mutability:  MutabilityReadWrite,
						Returned:    ReturnedDefault,
						Uniqueness:  UniquenessNone,
					},
					{
						Name:        primaryAttr,
						Type:        TypeBoolean,
						Description: descPrimary,
						Mutability:  MutabilityReadWrite,
						Returned:    ReturnedDefault,
						Uniqueness:  UniquenessNone,
					},
				},
			},
		},
	}
}

func CoreGroupSchema() *Schema {
	return &Schema{
		ID:          GroupSchemaURI,
		Name:        descGroupResource,
		Description: descGroupResource,
		Attributes: []Attribute{
			{
				Name:        idAttr,
				Type:        TypeString,
				Description: "A unique identifier for a SCIM resource as defined by the service provider.",
				CaseExact:   true,
				Mutability:  MutabilityReadOnly,
				Returned:    ReturnedAlways,
				Uniqueness:  UniquenessServer,
			},
			{
				Name:        externalIDAttr,
				Type:        TypeString,
				Description: "A String that is an identifier for the resource as defined by the provisioning client.",
				Mutability:  MutabilityReadWrite,
				Returned:    ReturnedDefault,
				Uniqueness:  UniquenessNone,
			},
			{
				Name:        displayNameAttr,
				Type:        TypeString,
				Description: "A human-readable name for the Group.",
				Required:    true,
				Mutability:  MutabilityReadWrite,
				Returned:    ReturnedDefault,
				Uniqueness:  UniquenessNone,
			},
			{
				Name:        "members",
				Type:        TypeComplex,
				MultiValued: true,
				Description: "A list of members of the Group.",
				Mutability:  MutabilityReadWrite,
				Returned:    ReturnedDefault,
				Uniqueness:  UniquenessNone,
				SubAttributes: []Attribute{
					{
						Name:        valueAttr,
						Type:        TypeString,
						Description: "Identifier of the member of this Group.",
						Mutability:  MutabilityImmutable,
						Returned:    ReturnedDefault,
						Uniqueness:  UniquenessNone,
					},
					{
						Name:           "$ref",
						Type:           TypeReference,
						Description:    "The URI corresponding to a SCIM resource that is a member of this Group.",
						Mutability:     MutabilityImmutable,
						Returned:       ReturnedDefault,
						Uniqueness:     UniquenessNone,
						ReferenceTypes: []string{descUserResource, groupRef},
					},
					{
						Name:        displayAttr,
						Type:        TypeString,
						Description: "A human-readable name for the member.",
						Mutability:  MutabilityReadOnly,
						Returned:    ReturnedDefault,
						Uniqueness:  UniquenessNone,
					},
					{
						Name:            typeAttr,
						Type:            TypeString,
						Description:     "A label indicating the type of resource, e.g., 'User' or 'Group'.",
						CanonicalValues: []string{descUserResource, groupRef},
						Mutability:      MutabilityImmutable,
						Returned:        ReturnedDefault,
						Uniqueness:      UniquenessNone,
					},
				},
			},
		},
	}
}

func EnterpriseUserSchema() *Schema {
	return &Schema{
		ID:          EnterpriseUserSchemaURI,
		Name:        "EnterpriseUser",
		Description: "Enterprise User",
		Attributes: []Attribute{
			{
				Name:        employeeNumberAttr,
				Type:        TypeString,
				Description: "Numeric or alphanumeric identifier assigned to a person, typically based on order of hire or association with an organization.",
				Mutability:  MutabilityReadWrite,
				Returned:    ReturnedDefault,
				Uniqueness:  UniquenessNone,
			},
			{
				Name:        "costCenter",
				Type:        TypeString,
				Description: "Identifies the name of a cost center.",
				Mutability:  MutabilityReadWrite,
				Returned:    ReturnedDefault,
				Uniqueness:  UniquenessNone,
			},
			{
				Name:        "organization",
				Type:        TypeString,
				Description: "Identifies the name of an organization.",
				Mutability:  MutabilityReadWrite,
				Returned:    ReturnedDefault,
				Uniqueness:  UniquenessNone,
			},
			{
				Name:        "division",
				Type:        TypeString,
				Description: "Identifies the name of a division.",
				Mutability:  MutabilityReadWrite,
				Returned:    ReturnedDefault,
				Uniqueness:  UniquenessNone,
			},
			{
				Name:        "department",
				Type:        TypeString,
				Description: "Identifies the name of a department.",
				Mutability:  MutabilityReadWrite,
				Returned:    ReturnedDefault,
				Uniqueness:  UniquenessNone,
			},
			{
				Name:        "manager",
				Type:        TypeComplex,
				Description: "The User's manager.",
				Mutability:  MutabilityReadWrite,
				Returned:    ReturnedDefault,
				Uniqueness:  UniquenessNone,
				SubAttributes: []Attribute{
					{
						Name:        valueAttr,
						Type:        TypeString,
						Description: "The id of the SCIM resource representing the User's manager.",
						Mutability:  MutabilityReadWrite,
						Returned:    ReturnedDefault,
						Uniqueness:  UniquenessNone,
					},
					{
						Name:           "$ref",
						Type:           TypeReference,
						Description:    "The URI of the SCIM resource representing the User's manager.",
						Mutability:     MutabilityReadWrite,
						Returned:       ReturnedDefault,
						Uniqueness:     UniquenessNone,
						ReferenceTypes: []string{descUserResource},
					},
					{
						Name:        displayNameAttr,
						Type:        TypeString,
						Description: "The displayName of the User's manager.",
						Mutability:  MutabilityReadOnly,
						Returned:    ReturnedDefault,
						Uniqueness:  UniquenessNone,
					},
				},
			},
		},
	}
}

func UserResourceType() *ResourceType {
	return &ResourceType{
		ID:          descUserResource,
		Name:        descUserResource,
		Description: "User Account",
		Endpoint:    usersEndpoint,
		Schema:      UserSchemaURI,
		SchemaExtensions: []SchemaExtension{
			{
				Schema:   EnterpriseUserSchemaURI,
				Required: false,
			},
		},
	}
}

func GroupResourceType() *ResourceType {
	return &ResourceType{
		ID:               groupRef,
		Name:             groupRef,
		Description:      groupRef,
		Endpoint:         groupsEndpoint,
		Schema:           GroupSchemaURI,
		SchemaExtensions: []SchemaExtension{},
	}
}

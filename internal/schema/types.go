package schema

type InformationSchema struct {
	Indexes      []*InformationSchemaIndex       `json:"INDEXES"`
	Tables       []*InformationSchemaTable       `json:"TABLES"`
	IndexColumns []*InformationSchemaIndexColumn `json:"INDEX_COLUMNS"`
	Columns      []*InformationSchemaColumn      `json:"COLUMNS"`
}

type InformationSchemaIndex struct {
	TableCatalog     string  `spanner:"TABLE_CATALOG" json:"TABLE_CATALOG"`
	TableSchema      string  `spanner:"TABLE_SCHEMA" json:"TABLE_SCHEMA"`
	TableName        string  `spanner:"TABLE_NAME" json:"TABLE_NAME"`
	IndexName        string  `spanner:"INDEX_NAME" json:"INDEX_NAME"`
	IndexType        string  `spanner:"INDEX_TYPE" json:"INDEX_TYPE"`
	ParentTableName  string  `spanner:"PARENT_TABLE_NAME" json:"PARENT_TABLE_NAME"`
	IsUnique         bool    `spanner:"IS_UNIQUE" json:"IS_UNIQUE"`
	IsNullFiltered   bool    `spanner:"IS_NULL_FILTERED" json:"IS_NULL_FILTERED"`
	IndexState       *string `spanner:"INDEX_STATE" json:"INDEX_STATE"`
	SpannerIsManaged bool    `spanner:"SPANNER_IS_MANAGED" json:"SPANNER_IS_MANAGED"`
}
type InformationSchemaIndexColumn struct {
	TableCatalog    string  `spanner:"TABLE_CATALOG" json:"TABLE_CATALOG"`
	TableSchema     string  `spanner:"TABLE_SCHEMA" json:"TABLE_SCHEMA"`
	TableName       string  `spanner:"TABLE_NAME" json:"TABLE_NAME"`
	IndexName       string  `spanner:"INDEX_NAME" json:"INDEX_NAME"`
	IndexType       string  `spanner:"INDEX_TYPE" json:"INDEX_TYPE"`
	ColumnName      string  `spanner:"COLUMN_NAME" json:"COLUMN_NAME"`
	OrdinalPosition *int64  `spanner:"ORDINAL_POSITION" json:"ORDINAL_POSITION"`
	ColumnOrdering  *string `spanner:"COLUMN_ORDERING" json:"COLUMN_ORDERING"`
	IsNullable      *string `spanner:"IS_NULLABLE" json:"IS_NULLABLE"`
	SpannerType     *string `spanner:"SPANNER_TYPE" json:"SPANNER_TYPE"`
}

type InformationSchemaColumn struct {
	TableCatalog         string  `spanner:"TABLE_CATALOG" json:"TABLE_CATALOG"`
	TableSchema          string  `spanner:"TABLE_SCHEMA" json:"TABLE_SCHEMA"`
	TableName            string  `spanner:"TABLE_NAME" json:"TABLE_NAME"`
	ColumnName           string  `spanner:"COLUMN_NAME" json:"COLUMN_NAME"`
	OrdinalPosition      int64   `spanner:"ORDINAL_POSITION" json:"ORDINAL_POSITION"`
	ColumnDefault        *string `spanner:"COLUMN_DEFAULT" json:"COLUMN_DEFAULT"`
	DataType             *string `spanner:"DATA_TYPE" json:"DATA_TYPE"`
	IsNullable           *string `spanner:"IS_NULLABLE" json:"IS_NULLABLE"`
	SpannerType          *string `spanner:"SPANNER_TYPE" json:"SPANNER_TYPE"`
	IsGenerated          string  `spanner:"IS_GENERATED" json:"IS_GENERATED"`
	GenerationExpression *string `spanner:"GENERATION_EXPRESSION" json:"GENERATION_EXPRESSION"`
	IsStored             *string `spanner:"IS_STORED" json:"IS_STORED"`
	SpannerState         *string `spanner:"SPANNER_STATE" json:"SPANNER_STATE"`
}

type InformationSchemaTable struct {
	TableCatalog                string  `spanner:"TABLE_CATALOG" json:"TABLE_CATALOG"`
	TableSchema                 string  `spanner:"TABLE_SCHEMA" json:"TABLE_SCHEMA"`
	TableName                   string  `spanner:"TABLE_NAME" json:"TABLE_NAME"`
	ParentTableName             *string `spanner:"PARENT_TABLE_NAME" json:"PARENT_TABLE_NAME"`
	OnDeleteAction              *string `spanner:"ON_DELETE_ACTION" json:"ON_DELETE_ACTION"`
	TableType                   string  `spanner:"TABLE_TYPE" json:"TABLE_TYPE"`
	SpannerState                *string `spanner:"SPANNER_STATE" json:"SPANNER_STATE"`
	InterleaveType              *string `spanner:"INTERLEAVE_TYPE" json:"INTERLEAVE_TYPE"`
	RowDeletionPolicyExpression *string `spanner:"ROW_DELETION_POLICY_EXPRESSION" json:"ROW_DELETION_POLICY_EXPRESSION"`
}

type InformationSchemaSchema struct {
	CatalogName        string  `spanner:"CATALOG_NAME" json:"CATALOG_NAME"`
	SchemaName         string  `spanner:"SCHEMA_NAME" json:"SCHEMA_NAME"`
	EffectiveTimestamp *int64  `spanner:"EFFECTIVE_TIMESTAMP" json:"EFFECTIVE_TIMESTAMP"`
	SchemaOwner        *string `spanner:"SCHEMA_OWNER" json:"SCHEMA_OWNER"`
}

type InformationSchemaDatabaseOption struct {
	CatalogName string `spanner:"CATALOG_NAME" json:"CATALOG_NAME"`
	SchemaName  string `spanner:"SCHEMA_NAME" json:"SCHEMA_NAME"`
	OptionName  string `spanner:"OPTION_NAME" json:"OPTION_NAME"`
	OptionType  string `spanner:"OPTION_TYPE" json:"OPTION_TYPE"`
	OptionValue string `spanner:"OPTION_VALUE" json:"OPTION_VALUE"`
}

type InformationSchemaColumnPrivilege struct {
	TableCatalog  string `spanner:"TABLE_CATALOG" json:"TABLE_CATALOG"`
	TableSchema   string `spanner:"TABLE_SCHEMA" json:"TABLE_SCHEMA"`
	TableName     string `spanner:"TABLE_NAME" json:"TABLE_NAME"`
	ColumnName    string `spanner:"COLUMN_NAME" json:"COLUMN_NAME"`
	PrivilegeType string `spanner:"PRIVILEGE_TYPE" json:"PRIVILEGE_TYPE"`
	Grantee       string `spanner:"GRANTEE" json:"GRANTEE"`
}

type InformationSchemaColumnOption struct {
	TableCatalog string `spanner:"TABLE_CATALOG" json:"TABLE_CATALOG"`
	TableSchema  string `spanner:"TABLE_SCHEMA" json:"TABLE_SCHEMA"`
	TableName    string `spanner:"TABLE_NAME" json:"TABLE_NAME"`
	ColumnName   string `spanner:"COLUMN_NAME" json:"COLUMN_NAME"`
	OptionName   string `spanner:"OPTION_NAME" json:"OPTION_NAME"`
	OptionType   string `spanner:"OPTION_TYPE" json:"OPTION_TYPE"`
	OptionValue  string `spanner:"OPTION_VALUE" json:"OPTION_VALUE"`
}

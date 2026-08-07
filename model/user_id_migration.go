package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"gorm.io/gorm"
)

func o023NormalizeSchemaSQL(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

// o023SchemaManifestText deterministically serializes every SQLite schema
// object plus the column, foreign-key, and index metadata that SQLite exposes.
// Audit compares this text with a code-owned versioned manifest; this reader
// never decides what is allowed.
func o023SchemaManifestText(db *gorm.DB) (string, error) {
	var objects []struct{ Type, Name, SQL string }
	if err := db.Raw("SELECT type, name, coalesce(sql, '') AS sql FROM sqlite_schema WHERE type IN ('table','view','trigger','index') ORDER BY type, name").Scan(&objects).Error; err != nil {
		return "", err
	}
	lines := make([]string, 0, len(objects)*4)
	tables := make([]string, 0)
	for _, object := range objects {
		if strings.HasPrefix(object.Name, "sqlite_") && object.Name != "sqlite_sequence" {
			continue
		}
		lines = append(lines, strings.Join([]string{"object", object.Type, object.Name, o023NormalizeSchemaSQL(object.SQL)}, "|"))
		if object.Type == "table" && object.Name != "sqlite_sequence" {
			tables = append(tables, object.Name)
		}
	}
	for _, table := range tables {
		var columns []struct {
			CID        int
			Name       string
			Type       string
			NotNull    int     `gorm:"column:notnull"`
			DefaultSQL *string `gorm:"column:dflt_value"`
			PrimaryKey int     `gorm:"column:pk"`
			Hidden     int
		}
		if err := db.Raw("SELECT cid, name, type, \"notnull\", dflt_value, pk, hidden FROM pragma_table_xinfo(?) ORDER BY cid", table).Scan(&columns).Error; err != nil {
			return "", err
		}
		for _, column := range columns {
			defaultSQL := "<nil>"
			if column.DefaultSQL != nil {
				defaultSQL = o023NormalizeSchemaSQL(*column.DefaultSQL)
			}
			lines = append(lines, fmt.Sprintf("column|%s|%d|%s|%s|%d|%s|%d|%d", table, column.CID, column.Name, strings.ToLower(column.Type), column.NotNull, defaultSQL, column.PrimaryKey, column.Hidden))
		}
		var foreignKeys []struct {
			ID, Seq                   int
			Parent, From, To          string
			OnUpdate, OnDelete, Match string
		}
		if err := db.Raw("SELECT id, seq, \"table\" AS parent, \"from\" AS \"from\", \"to\" AS \"to\", on_update, on_delete, match FROM pragma_foreign_key_list(?) ORDER BY id, seq", table).Scan(&foreignKeys).Error; err != nil {
			return "", err
		}
		for _, foreignKey := range foreignKeys {
			lines = append(lines, fmt.Sprintf("foreign_key|%s|%d|%d|%s|%s|%s|%s|%s|%s", table, foreignKey.ID, foreignKey.Seq, foreignKey.Parent, foreignKey.From, foreignKey.To, foreignKey.OnUpdate, foreignKey.OnDelete, foreignKey.Match))
		}
		var indexes []struct {
			Seq, Unique, Partial int
			Name, Origin         string
		}
		if err := db.Raw("SELECT seq, name, \"unique\" AS \"unique\", origin, partial FROM pragma_index_list(?) ORDER BY name", table).Scan(&indexes).Error; err != nil {
			return "", err
		}
		for _, index := range indexes {
			lines = append(lines, fmt.Sprintf("index|%s|%s|%d|%s|%d", table, index.Name, index.Unique, index.Origin, index.Partial))
			var indexColumns []struct {
				SeqNo, CID int
				Name       *string
				Desc, Key  int
				Collation  string `gorm:"column:coll"`
			}
			if err := db.Raw("SELECT seqno, cid, name, \"desc\" AS \"desc\", coll, \"key\" AS \"key\" FROM pragma_index_xinfo(?) ORDER BY seqno", index.Name).Scan(&indexColumns).Error; err != nil {
				return "", err
			}
			for _, indexColumn := range indexColumns {
				name := "<nil>"
				if indexColumn.Name != nil {
					name = *indexColumn.Name
				}
				lines = append(lines, fmt.Sprintf("index_column|%s|%s|%d|%d|%s|%d|%s|%d", table, index.Name, indexColumn.SeqNo, indexColumn.CID, name, indexColumn.Desc, indexColumn.Collation, indexColumn.Key))
			}
		}
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n"), nil
}

type o023SchemaManifestSummary struct {
	Digest        string
	ObjectCounts  map[string]int
	TableDigests  map[string]string
	IndexDigest   string
	TriggerDigest string
	ViewDigest    string
}

var o023CanonicalSchemaTableDigests = map[string]string{
	"abilities":                        "70e80ce2b027f0fa3a5233db085137fc6e6dc4a558b2bc735e877cc48e00632d",
	"auth_flows":                       "c86dfa142e40a449008eaea7b40fa2ae7006d27bad9b4792599204644336895e",
	"authz_roles":                      "0ca71842e7f9c13b64c08d3456117159cc67927f1828b84bea20b39a9505f916",
	"bonus_balances":                   "63cdc62ebbeaf73529df648cd0939a9f04f828c1901f10932eb064567733db8f",
	"casbin_rule":                      "5f72990437ea996bf766199527bdf547399b143570f2b6418a75f1303d2b71b0",
	"channels":                         "6829429895c89f827882dae57e5afc060992896c13ce5cce081dba85617615b5",
	"checkins":                         "5eae30ba5cb8dc53b6fc7af6dd7f37402562c6d2fce0915ec02ae5748c6ffc1a",
	"custom_oauth_providers":           "d8d9616f2feb3afd7d69f0543ff1621534940eb0269f24d66afd781146235198",
	"external_identity_claims":         "87e94c5045b5c08d8fddf6e00972020dcd54b83121e761e3684abc36addb3201",
	"logs":                             "635be13da2ae4dbc0cfb570f72ea442e76a9678c148a5e1b4867325faf3f47b9",
	"midjourneys":                      "197d8c6bab82d116f062fd5c23137c78bf993a5af03725f55975cb2ebde93741",
	"models":                           "32d87090ecf096260446d8527edecac201a66c96a301502add1d284efe537c69",
	"options":                          "2236729ea82b4f2de184cd78972f0fb83bc3b7c74c1a116a46ddb2018bd0cc16",
	"passkey_credentials":              "cfe300b8453fe81c6ba508d05c3cba15d79fddb3b211452ce90a081d11e94a6d",
	"payment_campaign_claims":          "657034b3e7072af97f40b2ff8a231289b0472c1ea6375c0b73ebf97be6e13a9d",
	"payment_campaign_participants":    "f20120ee052c0b4e578c7cb9902a10d431643b76f5641621fb597fb12cd86173",
	"payment_campaign_states":          "c790ef3863e3a8c1d3d6dbcdcf41780df19b7114794375c29b8e5c48367cb625",
	"perf_metrics":                     "53e18ed1e12747affdae8a9277efa8816f4c6fc014d3bb85ea60346e91300639",
	"prefill_groups":                   "442783236622e13dce60223e987f4e30d2ad93b37942fe603ff3d5813b2cd7c3",
	"quota_data":                       "965cb8ae60eacc504fd354a9297297e6ba20f1e66093324ba6a72718f9d4d725",
	"redemptions":                      "ab451a68b6ce70f9b4492a9f33fa6474b0d8976747cd00fe4430fa6c39bdede3",
	"setups":                           "88fd8f1a0694fc584890d8c4c27ac273cb5e7d57939e71830964fcb4ea74691c",
	"subscription_orders":              "fae6036c4f066367eabafb672f01d39c23f3beb77bca36032d9580a5d5f3a9c0",
	"subscription_plans":               "adbcc094360cf349e6fc2317d742243a806c210906ba0eb15194ae76dabc06a1",
	"subscription_pre_consume_records": "5e8691cc66c3a274dac7055b867aaf71bdad86452b9420bb2c494c6a8929fa8a",
	"system_instances":                 "d99dbcc97d7524e9ac69d9a610ef06f7e29f3bc43ead2714819ed2ed828f60c6",
	"system_task_locks":                "b82d56abab6b4cbe9f01903d88ed2acd33b4e04634a46c4c7872535a9515c38f",
	"system_tasks":                     "c1de2ce065824ebe9445cf9420f1983f95cc4fc07406336db6b9ae5708ff09a1",
	"tasks":                            "b3c79d29115887e2316309ad535e53c2970c1f7eda941e640a27406d7d3c4ac5",
	"tokens":                           "321bc89e85ed5e740bc8aa0cccb07317dcc2d50e66740dadec52f7b253e65302",
	"top_ups":                          "e1a2d3a555fc603b207d9251859e790fb6a53af24a7f9bb647fe45fa055432e2",
	"two_fa_backup_codes":              "93755496617720d3e8db304b430d51be9eac8f068a9deed004feba00d9580886",
	"two_fas":                          "af7719d392b1f09315a49652a352ad79041adf35e1ed81f14e82e1df214e6f44",
	"user_oauth_bindings":              "07680cd7dddb0ffa3613471a10aa2df2806cab7357d94d1d8f258903bf77bf7c",
	"user_sessions":                    "8f42c2fcc56096de5b9ec3b31f28e945e1622ad7183e48a36bbf86d653508a2f",
	"user_subscriptions":               "c584510e4e38a6e38ebc9ffe136dcdb36a28941a20224ac8dacfaf0ee8a52d0c",
	"users":                            "75027fccfd693215abc013f1cbfaa87a12fc5fb9d690a8827331d66cd9fd0c62",
	"vendors":                          "037fca0fbd0208bccaf5926a78162edc55b9923d36ced74755f2b3382e4df1d9",
	"wallet_consume_records":           "bda50caf3bc6c09550c6066f94600d2462260605ac2e730a7efc7dc3a661a6c4",
}

var o023ProductionLegacySchemaTableDigests = map[string]string{
	"bonus_balances":                "d3692e7455b3f8bebf49b2a818eed0eca33dbbee98744a26016f363f757fa603",
	"checkins":                      "1d5f2ebf9e095d9dfe968fc2fbd9aa5a6047660e62b663dbd2e8f18d5fdec95d",
	"external_identity_claims":      "5940ee33bf18281d8b56134f1e1a247d1f404c1a7a70d34777f494b546840be7",
	"logs":                          "eabddc78257afd9ef8b6aa736aa15545f700942af53e91f1e148520c28b90723",
	"models":                        "15de2925bd1641604b7f2512e1bb9f0561d30df02b18f8f7f0444a3293535324",
	"payment_campaign_claims":       "0f030e20c1c4cb3b325bd58649fcd1f25a3f8dcdf3f8989596e0b4f2f4dd0132",
	"payment_campaign_participants": "f3ad0332a2853ad409f0b83cab7ced5ce97d187d014cb4e41c7f1baf47c7c8a6",
	"payment_campaign_states":       "48490c468575153a913d95cf0aad727a94f062159366c771a6cf52bd0aa0bf3d",
	"perf_metrics":                  "01b3c6703e7892265b78ba6c599cfd838976ed36ef1974c9eaac3725f38c57a6",
	"quota_data":                    "c980d8e1d78b9139988596ed19cfffb291f9cef151ee805c57b39846ae3b9b9d",
	"subscription_orders":           "1729f12c5d82e1a8255f1ff8bb6f97d583bd5214180bd2511c37ebb67c4bc299",
	"subscription_plans":            "cda86d972c24408f73bfa190542e6355a59b172d48c9e30da1d72473d5b1957f",
	"tokens":                        "c2eccacd44d3843d17c6c183cd03c8e3d5433941adab5eed772262d3a99a8742",
	"top_ups":                       "8b2f702a97113832bec410dbe52bb4bbe61dc1f61fd7bbec96c9966a72e24937",
	"two_fas":                       "b3977ad2280e9e6cba6d4b2e4bc47e48affd467bfdb9d8044694820263a9e3ee",
	"user_oauth_bindings":           "1ac697ce7ed324940de5f2adbf54cf3658e00b7f86de830156227de375c84d31",
	"user_subscriptions":            "5431e53a3d19a797f708aaefdb0a5851c1db0664027c16f816e500d1f87928d9",
	"users":                         "5cf0ec4071792b4c5b386ea2a7fec3128a3c2a54505eb99e9c79bfc3036667b9",
	"vendors":                       "da29983b6452d4187534dec755ea9ab8a11be19c2a0a35b2cd097689e61d3d39",
}

const (
	o023SchemaLineageCanonical        = "canonical-v1"
	o023SchemaLineageProductionLegacy = "production-legacy-v1"
)

func o023CanonicalSchemaManifest(prepared bool) o023SchemaManifestSummary {
	tables := make(map[string]string, len(o023CanonicalSchemaTableDigests))
	for table, digest := range o023CanonicalSchemaTableDigests {
		tables[table] = digest
	}
	digest := "b38205d53060105c7bddb50a6e6e2f03889f06800e8018a6e8d6009049ffc0ae"
	if prepared {
		digest = "5d3cd734e058c389f2f3243003ba13605a90962c8c88b210f6660ef0155fc8fa"
		tables["top_ups"] = "7e3970edf73bde58c4fdc99429bd9cbe72b15cc359e307bd877c5ce7b4bf89d6"
		tables["subscription_orders"] = "c2625b87b99d734cd50b77df6122d6171427c53fcf1e9d71e7b8d8175d0f7edb"
	}
	return o023SchemaManifestSummary{
		Digest:        digest,
		ObjectCounts:  map[string]int{"table": 39, "index": 159, "trigger": 0, "view": 0},
		TableDigests:  tables,
		IndexDigest:   "12fd8f37929b1d1bc075de0ac7387652557903dbe8192dc89a169d10503b151a",
		TriggerDigest: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		ViewDigest:    "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
	}
}

func o023ProductionLegacySchemaManifest(prepared bool) o023SchemaManifestSummary {
	manifest := o023CanonicalSchemaManifest(false)
	for table, digest := range o023ProductionLegacySchemaTableDigests {
		manifest.TableDigests[table] = digest
	}
	manifest.Digest = "80b3cbc5ec7c494b6a4e6e652c4f56ba22681958aeb3a81494a8317d64712fc8"
	manifest.IndexDigest = "962878426f797a39781d804bfb8b6d695b0d84c4e89e89b1b9b0096b013b7b03"
	if prepared {
		manifest.Digest = "5ff24a649e90401cdfa0b2d1da058e59497ae53dbe485bb3b22c2b1221eec5af"
		manifest.TableDigests["top_ups"] = "e92bfe730aa6a3f2233fb78eafc5ae11d5211f16941b0d5ff00b562683f99960"
		manifest.TableDigests["subscription_orders"] = "ce8d520a83510e56df28eb326ba1fc0be2a189d6f17f201cf8ad35ceec462af3"
	}
	return manifest
}

func o023SchemaLineage(actual o023SchemaManifestSummary, prepared bool) (string, bool) {
	lineages := []struct {
		name     string
		manifest o023SchemaManifestSummary
	}{
		{name: o023SchemaLineageCanonical, manifest: o023CanonicalSchemaManifest(prepared)},
		{name: o023SchemaLineageProductionLegacy, manifest: o023ProductionLegacySchemaManifest(prepared)},
	}
	for _, lineage := range lineages {
		if o023SchemaManifestMatches(actual, lineage.manifest) {
			return lineage.name, true
		}
	}
	return "", false
}

func o023SchemaLineageFor(db *gorm.DB, prepared bool) (string, error) {
	actual, err := o023SchemaManifestSummaryFor(db)
	if err != nil {
		return "", err
	}
	lineage, ok := o023SchemaLineage(actual, prepared)
	if !ok {
		return "", fmt.Errorf("%w: canonical schema manifest mismatch", ErrUnknownUserReference)
	}
	return lineage, nil
}

func o023PersistedSchemaValue(lineage, fingerprint string) string {
	return strings.Join([]string{O023SchemaContractVersion, lineage, fingerprint}, "|")
}

func o023SchemaManifestMatches(actual, expected o023SchemaManifestSummary) bool {
	if actual.Digest != expected.Digest || actual.IndexDigest != expected.IndexDigest || actual.TriggerDigest != expected.TriggerDigest || actual.ViewDigest != expected.ViewDigest {
		return false
	}
	if len(actual.TableDigests) != len(expected.TableDigests) || len(actual.ObjectCounts) > len(expected.ObjectCounts) {
		return false
	}
	for objectType, count := range expected.ObjectCounts {
		if actual.ObjectCounts[objectType] != count {
			return false
		}
	}
	for table, digest := range expected.TableDigests {
		if actual.TableDigests[table] != digest {
			return false
		}
	}
	return true
}

func o023DigestLines(lines []string) string {
	sort.Strings(lines)
	digest := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(digest[:])
}

func o023SchemaManifestSummaryFor(db *gorm.DB) (o023SchemaManifestSummary, error) {
	text, err := o023SchemaManifestText(db)
	if err != nil {
		return o023SchemaManifestSummary{}, err
	}
	summary := o023SchemaManifestSummary{
		ObjectCounts: map[string]int{},
		TableDigests: map[string]string{},
	}
	tableLines := map[string][]string{}
	categoryLines := map[string][]string{"index": {}, "trigger": {}, "view": {}}
	for _, line := range strings.Split(text, "\n") {
		parts := strings.SplitN(line, "|", 4)
		if len(parts) < 3 {
			return o023SchemaManifestSummary{}, fmt.Errorf("invalid canonical manifest line")
		}
		switch parts[0] {
		case "object":
			summary.ObjectCounts[parts[1]]++
			if parts[1] == "table" {
				tableLines[parts[2]] = append(tableLines[parts[2]], line)
			} else if _, ok := categoryLines[parts[1]]; ok {
				categoryLines[parts[1]] = append(categoryLines[parts[1]], line)
			}
		case "column", "foreign_key", "index", "index_column":
			tableLines[parts[1]] = append(tableLines[parts[1]], line)
		default:
			return o023SchemaManifestSummary{}, fmt.Errorf("unknown canonical manifest line type %s", parts[0])
		}
	}
	summary.Digest = o023DigestLines(strings.Split(text, "\n"))
	for table, lines := range tableLines {
		summary.TableDigests[table] = o023DigestLines(lines)
	}
	summary.IndexDigest = o023DigestLines(categoryLines["index"])
	summary.TriggerDigest = o023DigestLines(categoryLines["trigger"])
	summary.ViewDigest = o023DigestLines(categoryLines["view"])
	return summary, nil
}

const (
	O023WaffoSnapshotVersion     = "o023_waffo_snapshot_version"
	O023UserIDMigrationVersion   = "o023_user_id_migration_version"
	O023SchemaHash               = "o023_schema_allowlist_hash"
	O023InvariantHash            = "o023_invariant_hash"
	O023SchemaContractVersion    = "o023-schema-v2"
	O023InvariantContractVersion = "o023-invariant-v2"
)

var (
	ErrUnknownUserReference     = errors.New("UNKNOWN_USER_REFERENCE")
	ErrUnmarkedPartialMigration = errors.New("UNMARKED_PARTIAL_MIGRATION")
	ErrMigrationStateCorrupt    = errors.New("MIGRATION_STATE_CORRUPT")
	ErrMigrationVersionConflict = errors.New("MIGRATION_VERSION_CONFLICT")
	ErrRedisTopologyUnverified  = errors.New("REDIS_TOPOLOGY_UNVERIFIED")
)

func o023ScanRedisKeyset(ctx context.Context) ([]string, error) {
	iterator := common.RDB.Scan(ctx, 0, "*", 0).Iterator()
	keys := make([]string, 0)
	seen := map[string]struct{}{}
	for iterator.Next(ctx) {
		key := iterator.Val()
		if _, duplicate := seen[key]; duplicate {
			return nil, ErrRedisTopologyUnverified
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	if err := iterator.Err(); err != nil {
		return nil, fmt.Errorf("%w: Redis SCAN failed", ErrRedisTopologyUnverified)
	}
	sort.Strings(keys)
	return keys, nil
}

// verifyO023RedisTopology proves that the configured client can enumerate one
// stable, complete logical Redis database. It performs no Redis write.
func verifyO023RedisTopology(ctx context.Context) error {
	if !common.RedisEnabled {
		return nil
	}
	if common.RDB == nil {
		return ErrRedisTopologyUnverified
	}
	if err := common.RDB.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("%w: Redis PING failed", ErrRedisTopologyUnverified)
	}
	before, err := common.RDB.DBSize(ctx).Result()
	if err != nil {
		return fmt.Errorf("%w: Redis DBSIZE failed", ErrRedisTopologyUnverified)
	}
	first, err := o023ScanRedisKeyset(ctx)
	if err != nil {
		return err
	}
	second, err := o023ScanRedisKeyset(ctx)
	if err != nil {
		return err
	}
	after, err := common.RDB.DBSize(ctx).Result()
	if err != nil {
		return fmt.Errorf("%w: Redis DBSIZE failed", ErrRedisTopologyUnverified)
	}
	if before != after || int64(len(first)) != before || len(first) != len(second) {
		return ErrRedisTopologyUnverified
	}
	for index := range first {
		if first[index] != second[index] {
			return ErrRedisTopologyUnverified
		}
	}
	return nil
}

func verifyO023RedisHasNoLegacyUserIDKeys(ctx context.Context) error {
	if !common.RedisEnabled {
		return nil
	}
	keys, err := o023ScanRedisKeyset(ctx)
	if err != nil {
		return err
	}
	for _, key := range keys {
		var candidate string
		switch {
		case strings.HasPrefix(key, "user:"):
			candidate = strings.TrimPrefix(key, "user:")
		case strings.HasPrefix(key, "auth:user:"):
			parts := strings.Split(key, ":")
			if len(parts) >= 4 {
				candidate = parts[len(parts)-1]
			}
		case strings.HasPrefix(key, "notify_limit:"):
			parts := strings.Split(key, ":")
			if len(parts) >= 2 {
				candidate = parts[1]
			}
		}
		if candidate == "" {
			continue
		}
		userID, parseErr := strconv.ParseInt(candidate, 10, 64)
		if parseErr == nil && (userID < int64(UserIDMin) || userID > int64(UserIDMax)) {
			return fmt.Errorf("%w: raw legacy Redis key", ErrUnknownUserReference)
		}
	}
	return nil
}

var o023MigrationFailureHook func(string) error

func invokeO023MigrationHook(stage string) error {
	if o023MigrationFailureHook == nil {
		return nil
	}
	return o023MigrationFailureHook(stage)
}

var o023DirectUserReferences = map[string][]string{
	"users": {"inviter_id"}, "tokens": {"user_id"}, "logs": {"user_id"},
	"midjourneys": {"user_id"}, "top_ups": {"user_id"}, "tasks": {"user_id"},
	"quota_data": {"user_id"}, "payment_campaign_claims": {"user_id"},
	"payment_campaign_participants": {"user_id"}, "bonus_balances": {"user_id"},
	"wallet_consume_records": {"user_id"}, "redemptions": {"user_id", "used_user_id"},
	"user_sessions": {"user_id"}, "auth_flows": {"user_id"},
	"external_identity_claims": {"user_id"}, "passkey_credentials": {"user_id"},
	"two_fas": {"user_id"}, "two_fa_backup_codes": {"user_id"},
	"subscription_orders": {"user_id"}, "user_subscriptions": {"user_id"},
	"subscription_pre_consume_records": {"user_id"}, "user_oauth_bindings": {"user_id"},
	"checkins": {"user_id"},
}

// These are the only persisted fields whose values can contain an O-023
// structured user reference. Keep this list explicit: schema discovery must
// never turn a newly added JSON/TEXT column into an implicit migration target.
var o023StructuredColumns = map[string][]string{
	"users": {"setting"},
	"tasks": {"data", "private_data", "properties"},
	"logs":  {"other"},
}

var o023RelationIdentityColumn = map[string]string{
	"user_sessions": "sid",
}

func sqliteTableExists(db *gorm.DB, table string) (bool, error) {
	var count int64
	err := db.Raw("SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = ?", table).Scan(&count).Error
	return count > 0, err
}

func sqliteColumnExists(db *gorm.DB, table, column string) (bool, error) {
	var count int64
	err := db.Raw("SELECT count(*) FROM pragma_table_xinfo(?) WHERE name = ?", table, column).Scan(&count).Error
	return count > 0, err
}

// PrepareO023Schema is the only creator of the Waffo snapshot columns.
func PrepareO023Schema(db *gorm.DB) error {
	if !common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		return fmt.Errorf("O-023 only supports SQLite: %w", ErrUnknownUserReference)
	}
	if err := AuditO023Schema(db); err != nil {
		return fmt.Errorf("O-023 schema audit before prepare: %w", err)
	}
	return db.Connection(func(conn *gorm.DB) error {
		if err := conn.Exec("BEGIN IMMEDIATE").Error; err != nil {
			return err
		}
		committed := false
		defer func() {
			if !committed {
				_ = conn.Exec("ROLLBACK").Error
			}
		}()
		lineage, err := o023SchemaLineageFor(conn, false)
		if err != nil {
			return err
		}
		for _, table := range []string{"top_ups", "subscription_orders"} {
			exists, err := sqliteTableExists(conn, table)
			if err != nil || !exists {
				if err != nil {
					return err
				}
				return fmt.Errorf("missing required table %s", table)
			}
			columnExists, err := sqliteColumnExists(conn, table, "waffo_buyer_identity")
			if err != nil {
				return err
			}
			if !columnExists {
				if err := conn.Exec("ALTER TABLE " + table + " ADD COLUMN waffo_buyer_identity varchar(128) NULL").Error; err != nil {
					return err
				}
			}
		}
		preparedLineage, err := o023SchemaLineageFor(conn, true)
		if err != nil {
			return err
		}
		if preparedLineage != lineage {
			return fmt.Errorf("%w: schema lineage changed during preparation", ErrMigrationVersionConflict)
		}
		hash, err := o023SchemaFingerprint(conn)
		if err != nil {
			return err
		}
		if err := conn.Exec("INSERT INTO options(key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value", O023SchemaHash, o023PersistedSchemaValue(lineage, hash)).Error; err != nil {
			return err
		}
		if err := conn.Exec("COMMIT").Error; err != nil {
			return err
		}
		committed = true
		return nil
	})
}

func o023OptionValue(tx *gorm.DB, key string) (string, bool, error) {
	var option Option
	err := tx.Where("key = ?", key).First(&option).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	return option.Value, err == nil, err
}

func setO023Option(tx *gorm.DB, key, value string) error {
	return tx.Save(&Option{Key: key, Value: value}).Error
}

func AuditO023Schema(db *gorm.DB) error {
	if !common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		return ErrUnknownUserReference
	}
	topUpPrepared, err := sqliteColumnExists(db, "top_ups", "waffo_buyer_identity")
	if err != nil {
		return err
	}
	subscriptionPrepared, err := sqliteColumnExists(db, "subscription_orders", "waffo_buyer_identity")
	if err != nil {
		return err
	}
	if topUpPrepared != subscriptionPrepared {
		return fmt.Errorf("%w: partial Waffo schema preparation", ErrUnknownUserReference)
	}
	lineage, err := o023SchemaLineageFor(db, topUpPrepared)
	if err != nil {
		return err
	}
	persisted, present, err := o023OptionValue(db, O023SchemaHash)
	if err != nil {
		return err
	}
	if topUpPrepared && !present {
		return fmt.Errorf("%w: canonical schema hash is missing", ErrMigrationVersionConflict)
	}
	if !topUpPrepared && present {
		return fmt.Errorf("%w: schema hash exists before preparation", ErrMigrationVersionConflict)
	}
	if present {
		fingerprint, err := o023SchemaFingerprint(db)
		if err != nil {
			return err
		}
		if persisted != o023PersistedSchemaValue(lineage, fingerprint) {
			return fmt.Errorf("%w: schema allowlist hash mismatch", ErrMigrationVersionConflict)
		}
	}
	return nil
}

func o023SchemaFingerprint(db *gorm.DB) (string, error) {
	var objects []struct{ Type, Name, SQL string }
	if err := db.Raw("SELECT type, name, coalesce(sql, '') AS sql FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%' ORDER BY type, name").Scan(&objects).Error; err != nil {
		return "", err
	}
	var builder strings.Builder
	for _, object := range objects {
		builder.WriteString(strings.ToLower(object.Type))
		builder.WriteByte('|')
		builder.WriteString(strings.ToLower(object.Name))
		builder.WriteByte('|')
		builder.WriteString(strings.ToLower(strings.Join(strings.Fields(object.SQL), " ")))
		builder.WriteByte('\n')
	}
	digest := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(digest[:]), nil
}

func O023MigrationStatus(db *gorm.DB) (string, error) {
	value, present, err := o023OptionValue(db, O023UserIDMigrationVersion)
	if err != nil {
		return "", err
	}
	var total, target int64
	if err := db.Model(&User{}).Unscoped().Count(&total).Error; err != nil {
		return "", err
	}
	if err := db.Model(&User{}).Unscoped().Where("id >= ? AND id <= ?", UserIDMin, UserIDMax).Count(&target).Error; err != nil {
		return "", err
	}
	if present {
		if value != "1" {
			return "", ErrMigrationVersionConflict
		}
		if target != total {
			return "", ErrMigrationStateCorrupt
		}
		return "ALREADY_MIGRATED", nil
	}
	if target != 0 && total != target {
		return "", ErrUnmarkedPartialMigration
	}
	return "READY", nil
}

func migrateO023References(tx *gorm.DB, table string, column string) error {
	exists, err := sqliteTableExists(tx, table)
	if err != nil || !exists {
		return err
	}
	columnExists, err := sqliteColumnExists(tx, table, column)
	if err != nil || !columnExists {
		return err
	}
	if table == "users" && column == "inviter_id" {
		return tx.Exec("UPDATE users AS u SET inviter_id = (SELECT new_id FROM temp_o023_user_ids WHERE old_id = u.inviter_id) WHERE u.inviter_id != 0").Error
	}
	return tx.Exec("UPDATE " + table + " SET " + column + " = (SELECT new_id FROM temp_o023_user_ids WHERE old_id = " + column + ") WHERE " + column + " != 0").Error
}

// migrateO023StructuredReferences only rewrites fields whose grammar is
// owned by Orbit. It deliberately does not run a textual replace across
// arbitrary payloads.
func migrateO023StructuredReferences(tx *gorm.DB) error {
	if exists, err := sqliteTableExists(tx, "casbin_rule"); err != nil {
		return err
	} else if exists {
		var rules []struct {
			ID uint
			V0 string
		}
		if err := tx.Table("casbin_rule").Where("v0 LIKE ?", "user:%").Find(&rules).Error; err != nil {
			return err
		}
		for _, rule := range rules {
			oldID, parseErr := strconv.Atoi(strings.TrimPrefix(rule.V0, "user:"))
			if parseErr != nil || oldID <= 0 {
				return fmt.Errorf("%w: invalid Casbin user reference %q", ErrUnknownUserReference, rule.V0)
			}
			mapped, err := o023MappedUserID(tx, oldID)
			if err != nil {
				return err
			}
			if err := tx.Table("casbin_rule").Where("id = ?", rule.ID).Update("v0", fmt.Sprintf("user:%d", mapped)).Error; err != nil {
				return err
			}
		}
	}
	if err := migrateO023CampaignKeys(tx); err != nil {
		return err
	}
	if exists, err := sqliteTableExists(tx, "options"); err != nil {
		return err
	} else if exists {
		var option Option
		err := tx.Where("key = ?", "payment_setting.compliance_confirmed_by").First(&option).Error
		if err == nil && strings.TrimSpace(option.Value) != "" {
			confirmedBy, parseErr := strconv.Atoi(strings.TrimSpace(option.Value))
			if parseErr != nil || confirmedBy <= 0 {
				return fmt.Errorf("%w: payment_setting.compliance_confirmed_by", ErrUnknownUserReference)
			}
			var mapped int
			if err := tx.Raw("SELECT new_id FROM temp_o023_user_ids WHERE old_id = ?", confirmedBy).Scan(&mapped).Error; err != nil {
				return err
			}
			if mapped == 0 {
				return fmt.Errorf("%w: payment_setting.compliance_confirmed_by=%d", ErrUnknownUserReference, confirmedBy)
			}
			if err := tx.Model(&Option{}).Where("key = ?", option.Key).Update("value", strconv.Itoa(mapped)).Error; err != nil {
				return err
			}
		} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}
	return scanO023StructuredPayloads(tx)
}

// migrateO023CampaignKeys only accepts the exact key grammars emitted by the
// campaign builders. Email claim and campaign-slot keys carry no user id and
// therefore remain byte-for-byte unchanged.
func migrateO023CampaignKeysLegacy(tx *gorm.DB) error {
	if exists, err := sqliteTableExists(tx, "payment_campaign_claims"); err != nil {
		return err
	} else if exists {
		column, err := sqliteColumnExists(tx, "payment_campaign_claims", "claim_key")
		if err != nil {
			return err
		}
		if column {
			if err := tx.Exec(`UPDATE payment_campaign_claims SET claim_key = CASE WHEN claim_key = campaign_id || ':user:' || user_id THEN campaign_id || ':user:' || (SELECT new_id FROM temp_o023_user_ids WHERE old_id = user_id) ELSE campaign_id || ':user:' || (SELECT new_id FROM temp_o023_user_ids WHERE old_id = user_id) || ':package:' || package_id END WHERE claim_key = campaign_id || ':user:' || user_id OR claim_key = campaign_id || ':user:' || user_id || ':package:' || package_id`).Error; err != nil {
				return err
			}
		}
	}
	if exists, err := sqliteTableExists(tx, "payment_campaign_participants"); err != nil {
		return err
	} else if exists {
		column, err := sqliteColumnExists(tx, "payment_campaign_participants", "participant_key")
		if err != nil {
			return err
		}
		if column {
			if err := tx.Exec(`UPDATE payment_campaign_participants SET participant_key = campaign_id || ':participant:user:' || (SELECT new_id FROM temp_o023_user_ids WHERE old_id = user_id) WHERE participant_key = campaign_id || ':participant:user:' || user_id`).Error; err != nil {
				return err
			}
		}
		column, err = sqliteColumnExists(tx, "payment_campaign_participants", "email_participant_key")
		if err != nil {
			return err
		}
		if column {
			if err := tx.Exec(`UPDATE payment_campaign_participants SET email_participant_key = campaign_id || ':user:' || (SELECT new_id FROM temp_o023_user_ids WHERE old_id = user_id) || ':email:' || email_hash || ':participant' WHERE email_participant_key = campaign_id || ':user:' || user_id || ':email:' || email_hash || ':participant'`).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func migrateO023CampaignKeys(tx *gorm.DB) error {
	// Validate and rewrite all key columns row-by-row. This makes an
	// unrecognized non-empty value a hard migration failure instead of a
	// silent UPDATE miss.
	var campaigns = make(map[string]operation_setting.PaymentCampaign)
	for _, campaign := range operation_setting.GetPaymentCampaigns() {
		campaigns[campaign.ID] = campaign
	}
	if exists, err := sqliteTableExists(tx, "payment_campaign_claims"); err != nil {
		return err
	} else if exists {
		var rows []struct {
			ID                                       int
			CampaignID, PackageID, EmailHash         string
			UserID                                   int
			ClaimKey, EmailClaimKey, CampaignSlotKey *string
		}
		if err := tx.Table("payment_campaign_claims").Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			campaign, known := campaigns[row.CampaignID]
			if err := validateO023ClaimKeys(row.CampaignID, row.PackageID, row.EmailHash, row.ClaimKey, row.EmailClaimKey, row.CampaignSlotKey, row.UserID, campaign, known); err != nil {
				return err
			}
			mapped, err := o023MappedUserID(tx, row.UserID)
			if err != nil {
				return err
			}
			if row.ClaimKey != nil && *row.ClaimKey != "" {
				if err := tx.Table("payment_campaign_claims").Where("id = ?", row.ID).Update("claim_key", o023RewriteClaimKey(*row.ClaimKey, row.CampaignID, row.UserID, mapped)).Error; err != nil {
					return err
				}
			}
		}
	}
	if exists, err := sqliteTableExists(tx, "payment_campaign_participants"); err != nil {
		return err
	} else if exists {
		var rows []struct {
			ID                                                   int
			CampaignID, EmailHash                                string
			UserID                                               int
			ParticipantKey, EmailParticipantKey, CampaignSlotKey *string
		}
		if err := tx.Table("payment_campaign_participants").Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			campaign, known := campaigns[row.CampaignID]
			if err := validateO023ParticipantKeys(row.CampaignID, row.EmailHash, row.ParticipantKey, row.EmailParticipantKey, row.CampaignSlotKey, row.UserID, campaign, known); err != nil {
				return err
			}
			mapped, err := o023MappedUserID(tx, row.UserID)
			if err != nil {
				return err
			}
			updates := map[string]any{}
			if row.ParticipantKey != nil && *row.ParticipantKey != "" {
				updates["participant_key"] = fmt.Sprintf("%s:participant:user:%d", row.CampaignID, mapped)
			}
			if row.EmailParticipantKey != nil && *row.EmailParticipantKey != "" && known && campaign.Eligibility == operation_setting.CampaignEligibilityPerCampaign {
				updates["email_participant_key"] = fmt.Sprintf("%s:user:%d:email:%s:participant", row.CampaignID, mapped, row.EmailHash)
			}
			if len(updates) != 0 {
				if err := tx.Table("payment_campaign_participants").Where("id = ?", row.ID).Updates(updates).Error; err != nil {
					return err
				}
			}
		}
	}
	return nil
}

var o023CampaignSlotPattern = regexp.MustCompile(`^[0-9]+$`)

func o023MappedUserID(tx *gorm.DB, oldID int) (int, error) {
	var mapped int
	if err := tx.Raw("SELECT new_id FROM temp_o023_user_ids WHERE old_id = ?", oldID).Scan(&mapped).Error; err != nil {
		return 0, err
	}
	if mapped == 0 {
		return 0, fmt.Errorf("%w: campaign user_id=%d", ErrUnknownUserReference, oldID)
	}
	return mapped, nil
}

func o023ValidSlotKey(prefix, key string) bool {
	if key == "" || !strings.HasPrefix(key, prefix+":slot:") {
		return false
	}
	slot := strings.TrimPrefix(key, prefix+":slot:")
	return o023CampaignSlotPattern.MatchString(slot) && slot != "0"
}

func o023ValidCampaignSlotKey(campaignID, key string) bool {
	prefix := campaignID + ":campaign-slot:"
	if !strings.HasPrefix(key, prefix) {
		return false
	}
	slot := strings.TrimPrefix(key, prefix)
	return o023CampaignSlotPattern.MatchString(slot) && slot != "0"
}

func o023ValidClaimKey(campaignID, packageID string, userID int, key string, campaign operation_setting.PaymentCampaign, known bool) bool {
	base := fmt.Sprintf("%s:user:%d", campaignID, userID)
	perPackage := fmt.Sprintf("%s:user:%d:package:%s", campaignID, userID, packageID)
	if known && campaign.Eligibility == operation_setting.CampaignEligibilityPerPackage {
		return o023ValidSlotKey(perPackage, key)
	}
	if known && campaign.Eligibility != operation_setting.CampaignEligibilityPerPackage {
		return o023ValidSlotKey(base, key)
	}
	return o023ValidSlotKey(base, key) || o023ValidSlotKey(perPackage, key)
}

func o023ValidEmailClaimKey(campaignID, packageID, emailHash, key string, campaign operation_setting.PaymentCampaign, known bool) bool {
	base := fmt.Sprintf("%s:email:%s", campaignID, emailHash)
	perPackage := fmt.Sprintf("%s:package:%s", base, packageID)
	legacyPerPackage := fmt.Sprintf("%s:package:%s:email:%s", campaignID, packageID, emailHash)
	if o023ValidSlotKey(legacyPerPackage, key) {
		return true
	}
	if known && campaign.Eligibility == operation_setting.CampaignEligibilityPerPackage {
		return o023ValidSlotKey(perPackage, key)
	}
	if known && campaign.Eligibility != operation_setting.CampaignEligibilityPerPackage {
		return o023ValidSlotKey(base, key)
	}
	return o023ValidSlotKey(base, key) || o023ValidSlotKey(perPackage, key)
}

func validateO023ClaimKeys(campaignID, packageID, emailHash string, claim, emailClaim, campaignSlot *string, userID int, campaign operation_setting.PaymentCampaign, known bool) error {
	if !known && ((claim != nil && *claim != "") || (emailClaim != nil && *emailClaim != "") || (campaignSlot != nil && *campaignSlot != "")) {
		return fmt.Errorf("%w: unknown campaign configuration %s", ErrMigrationStateCorrupt, campaignID)
	}
	if claim != nil && *claim != "" && !o023ValidClaimKey(campaignID, packageID, userID, *claim, campaign, known) {
		return fmt.Errorf("%w: invalid claim_key for campaign %s", ErrMigrationStateCorrupt, campaignID)
	}
	if emailClaim != nil && *emailClaim != "" && !o023ValidEmailClaimKey(campaignID, packageID, emailHash, *emailClaim, campaign, known) {
		return fmt.Errorf("%w: invalid email_claim_key for campaign %s", ErrMigrationStateCorrupt, campaignID)
	}
	if campaignSlot != nil && *campaignSlot != "" && !o023ValidCampaignSlotKey(campaignID, *campaignSlot) {
		return fmt.Errorf("%w: invalid campaign_slot_key for campaign %s", ErrMigrationStateCorrupt, campaignID)
	}
	if userID <= 0 {
		return fmt.Errorf("%w: invalid campaign user_id", ErrUnknownUserReference)
	}
	return nil
}

func validateO023ParticipantKeys(campaignID, emailHash string, participant, emailParticipant, campaignSlot *string, userID int, campaign operation_setting.PaymentCampaign, known bool) error {
	if !known && ((participant != nil && *participant != "") || (emailParticipant != nil && *emailParticipant != "") || (campaignSlot != nil && *campaignSlot != "")) {
		return fmt.Errorf("%w: unknown campaign configuration %s", ErrMigrationStateCorrupt, campaignID)
	}
	if participant != nil && *participant != "" && *participant != fmt.Sprintf("%s:participant:user:%d", campaignID, userID) {
		return fmt.Errorf("%w: invalid participant_key for campaign %s", ErrMigrationStateCorrupt, campaignID)
	}
	if emailParticipant != nil && *emailParticipant != "" {
		perCampaign := fmt.Sprintf("%s:user:%d:email:%s:participant", campaignID, userID, emailHash)
		perEmail := fmt.Sprintf("%s:email:%s:participant", campaignID, emailHash)
		valid := *emailParticipant == perEmail
		if known && campaign.Eligibility == operation_setting.CampaignEligibilityPerCampaign {
			valid = *emailParticipant == perCampaign
		}
		if !valid {
			return fmt.Errorf("%w: invalid email_participant_key for campaign %s", ErrMigrationStateCorrupt, campaignID)
		}
	}
	if campaignSlot != nil && *campaignSlot != "" && !o023ValidCampaignSlotKey(campaignID, *campaignSlot) {
		return fmt.Errorf("%w: invalid campaign_slot_key for campaign %s", ErrMigrationStateCorrupt, campaignID)
	}
	return nil
}

func o023RewriteClaimKey(key, campaignID string, oldID, newID int) string {
	oldPrefix := fmt.Sprintf("%s:user:%d", campaignID, oldID)
	return fmt.Sprintf("%s:user:%d", campaignID, newID) + strings.TrimPrefix(key, oldPrefix)
}

func scanO023StructuredPayloads(tx *gorm.DB) error {
	var pairs []struct {
		Old int `gorm:"column:old_id"`
		New int `gorm:"column:new_id"`
	}
	if err := tx.Raw("SELECT old_id, new_id FROM temp_o023_user_ids").Scan(&pairs).Error; err != nil {
		return err
	}
	mapping := make(map[int]int, len(pairs))
	for _, pair := range pairs {
		mapping[pair.Old] = pair.New
	}
	for table, columns := range o023StructuredColumns {
		exists, err := sqliteTableExists(tx, table)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		for _, column := range columns {
			ok, err := sqliteColumnExists(tx, table, column)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("%w: missing structured field %s.%s", ErrUnknownUserReference, table, column)
			}
			var rows []struct {
				RowID int64
				Value string
			}
			if err := tx.Raw("SELECT rowid AS row_id, \"" + column + "\" AS value FROM \"" + table + "\" WHERE \"" + column + "\" IS NOT NULL AND \"" + column + "\" <> ''").Scan(&rows).Error; err != nil {
				return fmt.Errorf("%w: structured field %s.%s: %v", ErrUnknownUserReference, table, column, err)
			}
			for _, row := range rows {
				updated, changed, err := rewriteO023JSON(row.Value, mapping)
				if err != nil {
					return fmt.Errorf("%w: invalid structured JSON in %s.%s: %v", ErrUnknownUserReference, table, column, err)
				}
				if changed {
					if err := tx.Exec("UPDATE \""+table+"\" SET \""+column+"\" = ? WHERE rowid = ?", updated, row.RowID).Error; err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func rewriteO023JSON(raw string, mapping map[int]int) (string, bool, error) {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return raw, false, err
	}
	if err := validateO023JSONValue(value, mapping); err != nil {
		return raw, false, err
	}
	changed := rewriteO023JSONValue(value, mapping)
	if !changed {
		return raw, false, nil
	}
	encoded, err := json.Marshal(value)
	return string(encoded), true, err
}

var o023StructuredUserReferenceKeys = map[string]struct{}{
	"admin_id":       {},
	"confirmed_by":   {},
	"inviter_id":     {},
	"target_user_id": {},
	"user_id":        {},
}

var o023UserActionsWithIDParam = map[string]struct{}{
	"user.delete":        {},
	"user.manage":        {},
	"user.reset_passkey": {},
	"user.update":        {},
}

var o023AuditRoutesWithUserIDParam = map[string]struct{}{
	"/api/subscription/admin/users/:id/subscriptions":       {},
	"/api/subscription/admin/users/:id/subscriptions/reset": {},
	"/api/user/:id":                             {},
	"/api/user/:id/2fa":                         {},
	"/api/user/:id/bindings/:binding_type":      {},
	"/api/user/:id/oauth/bindings/:provider_id": {},
	"/api/user/:id/reset_passkey":               {},
}

func o023IsStructuredUserReferenceKey(key string) bool {
	_, ok := o023StructuredUserReferenceKeys[strings.ToLower(key)]
	return ok
}

func o023StructuredUserID(value any) (int, error) {
	switch typed := value.(type) {
	case float64:
		if typed != float64(int(typed)) {
			return 0, fmt.Errorf("non-integral structured user reference")
		}
		return int(typed), nil
	case string:
		parsed, err := strconv.Atoi(typed)
		if err != nil {
			return 0, fmt.Errorf("invalid structured user reference")
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("unsupported structured user reference type")
	}
}

func o023ValidateMappedUserID(value any, mapping map[int]int) error {
	id, err := o023StructuredUserID(value)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnknownUserReference, err)
	}
	if _, ok := mapping[id]; !ok {
		return fmt.Errorf("%w: structured user reference=%d", ErrUnknownUserReference, id)
	}
	return nil
}

func o023RewriteMappedUserID(value any, mapping map[int]int) (any, bool) {
	id, err := o023StructuredUserID(value)
	if err != nil {
		return value, false
	}
	next, ok := mapping[id]
	if !ok {
		return value, false
	}
	if _, wasString := value.(string); wasString {
		return strconv.Itoa(next), true
	}
	return next, true
}

func o023UserActionIDParam(item map[string]any) (map[string]any, bool) {
	action, ok := item["action"].(string)
	if !ok {
		return nil, false
	}
	if _, ok := o023UserActionsWithIDParam[action]; !ok {
		return nil, false
	}
	params, ok := item["params"].(map[string]any)
	if !ok {
		return nil, false
	}
	if _, ok := params["id"]; !ok {
		return nil, false
	}
	return params, true
}

func o023AuditRouteUserIDParam(item map[string]any) (map[string]any, bool) {
	auditInfo, ok := item["audit_info"].(map[string]any)
	if !ok {
		return nil, false
	}
	route, ok := auditInfo["route"].(string)
	if !ok {
		return nil, false
	}
	if _, ok := o023AuditRoutesWithUserIDParam[route]; !ok {
		return nil, false
	}
	params, ok := auditInfo["params"].(map[string]any)
	if !ok {
		return nil, false
	}
	if _, ok := params["id"]; !ok {
		return nil, false
	}
	return params, true
}

func validateO023JSONValue(value any, mapping map[int]int) error {
	switch item := value.(type) {
	case []any:
		for _, child := range item {
			if err := validateO023JSONValue(child, mapping); err != nil {
				return err
			}
		}
	case map[string]any:
		if params, ok := o023UserActionIDParam(item); ok {
			if err := o023ValidateMappedUserID(params["id"], mapping); err != nil {
				return err
			}
		}
		if params, ok := o023AuditRouteUserIDParam(item); ok {
			if err := o023ValidateMappedUserID(params["id"], mapping); err != nil {
				return err
			}
		}
		for key, child := range item {
			if o023IsStructuredUserReferenceKey(key) {
				if err := o023ValidateMappedUserID(child, mapping); err != nil {
					return err
				}
			}
			if err := validateO023JSONValue(child, mapping); err != nil {
				return err
			}
		}
	}
	return nil
}

func rewriteO023JSONValue(value any, mapping map[int]int) bool {
	changed := false
	switch item := value.(type) {
	case []any:
		for _, child := range item {
			changed = rewriteO023JSONValue(child, mapping) || changed
		}
	case map[string]any:
		if params, ok := o023UserActionIDParam(item); ok {
			if next, rewritten := o023RewriteMappedUserID(params["id"], mapping); rewritten {
				params["id"] = next
				changed = true
			}
		}
		if params, ok := o023AuditRouteUserIDParam(item); ok {
			if next, rewritten := o023RewriteMappedUserID(params["id"], mapping); rewritten {
				params["id"] = next
				changed = true
			}
		}
		for key, child := range item {
			if o023IsStructuredUserReferenceKey(key) {
				if next, rewritten := o023RewriteMappedUserID(child, mapping); rewritten {
					item[key] = next
					changed = true
				}
			}
			changed = rewriteO023JSONValue(child, mapping) || changed
		}
	}
	return changed
}

func VerifyO023Invariants(db *gorm.DB) error {
	var invalid int64
	if err := db.Model(&User{}).Unscoped().Where("id < ? OR id > ? OR id = 0", UserIDMin, UserIDMax).Count(&invalid).Error; err != nil {
		return err
	}
	if invalid != 0 {
		return fmt.Errorf("invalid user id count: %d", invalid)
	}
	for table, columns := range o023DirectUserReferences {
		exists, err := sqliteTableExists(db, table)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		for _, column := range columns {
			query := "SELECT count(*) FROM \"" + table + "\" AS ref WHERE ref.\"" + column + "\" != 0 AND ref.\"" + column + "\" IS NOT NULL AND NOT EXISTS (SELECT 1 FROM users AS target WHERE target.id = ref.\"" + column + "\")"
			if err := db.Raw(query).Scan(&invalid).Error; err != nil {
				return err
			}
			if invalid != 0 {
				return fmt.Errorf("orphan reference %s.%s: %d", table, column, invalid)
			}
		}
	}
	if err := verifyO023SQLiteIntegrity(db); err != nil {
		return err
	}
	if err := verifyO023PostMigrationStructuredReferences(db); err != nil {
		return err
	}
	if err := verifyO023BusinessAggregates(db); err != nil {
		return err
	}
	return nil
}

func verifyO023PostMigrationStructuredReferences(db *gorm.DB) error {
	if exists, err := sqliteTableExists(db, "casbin_rule"); err != nil {
		return err
	} else if exists {
		var rows []struct{ V0 string }
		if err := db.Table("casbin_rule").Select("v0").Where("v0 LIKE ?", "user:%").Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			id, err := strconv.Atoi(strings.TrimPrefix(row.V0, "user:"))
			if err != nil || int64(id) < int64(UserIDMin) || int64(id) > int64(UserIDMax) {
				return fmt.Errorf("%w: old Casbin user reference %q", ErrUnknownUserReference, row.V0)
			}
		}
	}
	for _, table := range []string{"payment_campaign_claims", "payment_campaign_participants"} {
		exists, err := sqliteTableExists(db, table)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		for _, column := range []string{"claim_key", "participant_key", "email_participant_key"} {
			if ok, err := sqliteColumnExists(db, table, column); err != nil {
				return err
			} else if !ok {
				continue
			}
			var values []string
			if err := db.Table(table).Where(column+" IS NOT NULL AND "+column+" <> ''").Pluck(column, &values).Error; err != nil {
				return err
			}
			for _, value := range values {
				parts := strings.Split(value, ":")
				for i := 0; i+1 < len(parts); i++ {
					if parts[i] == "user" {
						id, err := strconv.Atoi(parts[i+1])
						if err != nil || int64(id) < int64(UserIDMin) || int64(id) > int64(UserIDMax) {
							return fmt.Errorf("%w: old activity user reference %q", ErrUnknownUserReference, value)
						}
					}
				}
			}
		}
	}
	if exists, err := sqliteTableExists(db, "options"); err != nil {
		return err
	} else if exists {
		var option Option
		err := db.Where("key = ?", "payment_setting.compliance_confirmed_by").First(&option).Error
		if err == nil {
			id, parseErr := strconv.Atoi(strings.TrimSpace(option.Value))
			if parseErr != nil || int64(id) < int64(UserIDMin) || int64(id) > int64(UserIDMax) {
				return fmt.Errorf("%w: old compliance user reference", ErrUnknownUserReference)
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}
	for table, columns := range o023StructuredColumns {
		exists, err := sqliteTableExists(db, table)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		for _, column := range columns {
			ok, err := sqliteColumnExists(db, table, column)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("%w: missing structured field %s.%s", ErrUnknownUserReference, table, column)
			}
			var values []string
			if err := db.Table(table).Where(column+" IS NOT NULL AND "+column+" <> ''").Pluck(column, &values).Error; err != nil {
				return err
			}
			for _, raw := range values {
				var value any
				if err := json.Unmarshal([]byte(raw), &value); err != nil {
					return fmt.Errorf("%w: invalid residual JSON %s.%s: %v", ErrUnknownUserReference, table, column, err)
				}
				if err := rejectO023LegacyJSONValue(value); err != nil {
					return fmt.Errorf("%w: residual structured reference %s.%s: %v", ErrUnknownUserReference, table, column, err)
				}
			}
		}
	}
	return nil
}

// verifyO023RawResiduals is intentionally independent from the equivalence
// snapshot. Direct user-reference columns and exact Redis namespaces are
// checked as raw values. Structured JSON and campaign keys are checked by
// verifyO023PostMigrationStructuredReferences using their explicit user-id
// grammar; scanning every number in those payloads would misclassify unrelated
// channel, token, quota, and slot identifiers as legacy user references.
func verifyO023RawResiduals(db *gorm.DB, legacyIDs []int) error {
	for table, columns := range o023DirectUserReferences {
		exists, err := sqliteTableExists(db, table)
		if err != nil || !exists {
			if err != nil {
				return err
			}
			continue
		}
		for _, column := range columns {
			ok, err := sqliteColumnExists(db, table, column)
			if err != nil || !ok {
				if err != nil {
					return err
				}
				continue
			}
			var values []int
			if err := db.Table(table).Where("\""+column+"\" IS NOT NULL AND \""+column+"\" != 0").Pluck(column, &values).Error; err != nil {
				return err
			}
			for _, value := range values {
				if int64(value) < int64(UserIDMin) || int64(value) > int64(UserIDMax) {
					return fmt.Errorf("%w: raw legacy ID at %s.%s", ErrUnknownUserReference, table, column)
				}
			}
		}
	}
	if err := verifyO023PostMigrationStructuredReferences(db); err != nil {
		return err
	}
	return verifyO023RawRedisResiduals(context.Background(), legacyIDs)
}

func verifyO023RawRedisResiduals(ctx context.Context, legacyIDs []int) error {
	if !common.RedisEnabled {
		return nil
	}
	if err := verifyO023RedisTopology(ctx); err != nil {
		return err
	}
	for _, id := range legacyIDs {
		patterns := []string{
			getUserCacheKey(id), getUserAuthFenceKey(id), getUserAuthVersionKey(id),
			fmt.Sprintf("auth:user:*:%d", id), fmt.Sprintf("notify_limit:%d:*", id),
		}
		for _, pattern := range patterns {
			if strings.Contains(pattern, "*") {
				it := common.RDB.Scan(ctx, 0, pattern, 0).Iterator()
				for it.Next(ctx) {
					return fmt.Errorf("%w: raw legacy Redis key", ErrUnknownUserReference)
				}
				if err := it.Err(); err != nil {
					return err
				}
			} else if exists, err := common.RDB.Exists(ctx, pattern).Result(); err != nil {
				return err
			} else if exists != 0 {
				return fmt.Errorf("%w: raw legacy Redis key", ErrUnknownUserReference)
			}
		}
	}
	return nil
}

func rejectO023LegacyJSONValue(value any) error {
	switch item := value.(type) {
	case []any:
		for _, child := range item {
			if err := rejectO023LegacyJSONValue(child); err != nil {
				return err
			}
		}
	case map[string]any:
		if params, ok := o023UserActionIDParam(item); ok {
			if err := rejectO023LegacyUserID(params["id"]); err != nil {
				return err
			}
		}
		if params, ok := o023AuditRouteUserIDParam(item); ok {
			if err := rejectO023LegacyUserID(params["id"]); err != nil {
				return err
			}
		}
		for key, child := range item {
			if o023IsStructuredUserReferenceKey(key) {
				if err := rejectO023LegacyUserID(child); err != nil {
					return err
				}
			}
			if err := rejectO023LegacyJSONValue(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func rejectO023LegacyUserID(value any) error {
	id, err := o023StructuredUserID(value)
	if err != nil {
		return err
	}
	if int64(id) < int64(UserIDMin) || int64(id) > int64(UserIDMax) {
		return fmt.Errorf("legacy user id %d", id)
	}
	return nil
}

// o023InvariantSnapshot is deliberately aggregate-only: it proves preservation
// without retaining the temporary old->new mapping in options or logs.
type o023InvariantSnapshot struct {
	Users, Quota, UsedQuota, Requests int64
	Facts                             map[string]string
	Relations                         map[string]string
	APIKeyCount                       int64
	APIKeyDigest                      string
	SessionCount, ActiveSessions      int64
	StructuredDigest, CacheDigest     string
	HistoricalWaffoDigest             map[string]o023HistoricalWaffoSnapshot
}

type o023HistoricalWaffoSnapshot struct {
	Boundary int64  `json:"boundary"`
	Count    int64  `json:"count"`
	Digest   string `json:"digest"`
}

type o023PersistedInvariant struct {
	Version         string                                 `json:"version"`
	Digest          string                                 `json:"digest"`
	HistoricalWaffo map[string]o023HistoricalWaffoSnapshot `json:"historical_waffo"`
}

func o023InvariantSnapshotFor(db *gorm.DB, mapping map[int]int) (o023InvariantSnapshot, error) {
	return o023InvariantSnapshotForBoundaries(db, mapping, nil)
}

func o023InvariantSnapshotForBoundaries(db *gorm.DB, mapping map[int]int, boundaries map[string]int64) (o023InvariantSnapshot, error) {
	var snapshot o023InvariantSnapshot
	snapshot.Facts = map[string]string{}
	snapshot.Relations = map[string]string{}
	snapshot.HistoricalWaffoDigest = map[string]o023HistoricalWaffoSnapshot{}
	if err := db.Model(&User{}).Unscoped().Count(&snapshot.Users).Error; err != nil {
		return snapshot, err
	}
	for name, dest := range map[string]*int64{"quota": &snapshot.Quota, "used_quota": &snapshot.UsedQuota, "request_count": &snapshot.Requests} {
		if err := db.Table("users").Select("coalesce(sum(" + name + "), 0)").Scan(dest).Error; err != nil {
			return snapshot, err
		}
	}
	for _, table := range []string{"top_ups", "payment_campaign_claims", "payment_campaign_participants", "bonus_balances", "wallet_consume_records", "subscription_orders", "user_subscriptions", "subscription_pre_consume_records", "user_oauth_bindings", "external_identity_claims", "two_fas", "two_fa_backup_codes", "checkins", "tasks", "logs"} {
		exists, err := sqliteTableExists(db, table)
		if err != nil {
			return snapshot, err
		}
		if !exists {
			continue
		}
		var count int64
		if err := db.Table(table).Count(&count).Error; err != nil {
			return snapshot, err
		}
		parts := []string{strconv.FormatInt(count, 10)}
		for _, column := range []string{"money", "amount", "credit_quota", "bonus_credit_quota", "quota", "bonus_quota", "wallet_quota", "quota_awarded", "amount_total", "amount_used", "used_quota", "refund_amount", "credit_amount", "pre_consumed"} {
			exists, err := sqliteColumnExists(db, table, column)
			if err != nil {
				return snapshot, err
			}
			if !exists {
				continue
			}
			var sum string
			if err := db.Table(table).Select("coalesce(sum(" + column + "), 0)").Scan(&sum).Error; err != nil {
				return snapshot, err
			}
			parts = append(parts, column+"="+sum)
		}
		snapshot.Facts[table] = strings.Join(parts, ";")
	}
	for table, columns := range o023DirectUserReferences {
		exists, err := sqliteTableExists(db, table)
		if err != nil {
			return snapshot, err
		}
		if !exists {
			continue
		}
		for _, column := range columns {
			exists, err := sqliteColumnExists(db, table, column)
			if err != nil {
				return snapshot, err
			}
			if !exists {
				continue
			}
			var values []struct {
				RowID string `gorm:"column:row_id"`
				Value int    `gorm:"column:value"`
			}
			identityColumn := "id"
			if configured, ok := o023RelationIdentityColumn[table]; ok {
				identityColumn = configured
			}
			if err := db.Table(table).Select(identityColumn + " AS row_id, " + column + " AS value").Where(column + " IS NOT NULL AND " + column + " != 0").Scan(&values).Error; err != nil {
				return snapshot, err
			}
			parts := make([]string, 0, len(values))
			for _, relation := range values {
				value := relation.Value
				if next, ok := mapping[value]; ok {
					value = next
				}
				identity := relation.RowID
				if parsed, err := strconv.Atoi(identity); err == nil {
					if next, ok := mapping[parsed]; ok {
						identity = strconv.Itoa(next)
					}
				}
				parts = append(parts, identity+"="+strconv.Itoa(value))
			}
			sort.Strings(parts)
			snapshot.Relations[table+"."+column] = strings.Join(parts, ",")
		}
	}
	if exists, err := sqliteTableExists(db, "tokens"); err != nil {
		return snapshot, err
	} else if exists {
		if err := db.Table("tokens").Count(&snapshot.APIKeyCount).Error; err != nil {
			return snapshot, err
		}
		if err := o023APISummaryInto(db, &snapshot.APIKeyDigest); err != nil {
			return snapshot, err
		}
	}
	if exists, err := sqliteTableExists(db, "user_sessions"); err != nil {
		return snapshot, err
	} else if exists {
		if err := db.Table("user_sessions").Count(&snapshot.SessionCount).Error; err != nil {
			return snapshot, err
		}
		if err := db.Table("user_sessions").Where("status != ?", "revoked").Count(&snapshot.ActiveSessions).Error; err != nil {
			return snapshot, err
		}
	}
	var err error
	if snapshot.StructuredDigest, err = o023StructuredDigest(db, mapping); err != nil {
		return snapshot, err
	}
	if snapshot.CacheDigest, err = o023CacheDigest(db, mapping); err != nil {
		return snapshot, err
	}
	if snapshot.HistoricalWaffoDigest, err = o023HistoricalWaffoDigests(db, boundaries); err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

// o023HistoricalWaffoDigests records the immutable, pre-migration buyer
// identity evidence. It is deliberately keyed by the stable order identity,
// provider, and raw bytes; it never uses the temporary user mapping.
func o023HistoricalWaffoDigests(db *gorm.DB, boundaries map[string]int64) (map[string]o023HistoricalWaffoSnapshot, error) {
	result := map[string]o023HistoricalWaffoSnapshot{}
	for _, table := range []string{"top_ups", "subscription_orders"} {
		exists, err := sqliteTableExists(db, table)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		hasSnapshot := true
		for _, column := range []string{"trade_no", "payment_provider", "waffo_buyer_identity"} {
			ok, err := sqliteColumnExists(db, table, column)
			if err != nil {
				return nil, err
			}
			if !ok {
				if column == "waffo_buyer_identity" {
					hasSnapshot = false
					break
				}
				return nil, fmt.Errorf("%w: missing Waffo identity key %s.%s", ErrUnknownUserReference, table, column)
			}
		}
		if !hasSnapshot {
			if boundaries != nil {
				return nil, fmt.Errorf("%w: missing historical Waffo snapshot column", ErrMigrationStateCorrupt)
			}
			continue
		}
		boundary, fixedBoundary := boundaries[table]
		if boundaries == nil || !fixedBoundary {
			if err := db.Raw("SELECT coalesce(max(id), 0) FROM " + table).Row().Scan(&boundary); err != nil {
				return nil, err
			}
		}
		var rows []struct {
			ID       int64
			TradeNo  string
			Provider string
			Identity *string
		}
		if err := db.Table(table).
			Select("id, trade_no, payment_provider AS provider, waffo_buyer_identity AS identity").
			Where("id <= ? AND (payment_provider = ? OR waffo_buyer_identity IS NOT NULL AND waffo_buyer_identity <> '')", boundary, PaymentProviderWaffoPancake).
			Order("id").Scan(&rows).Error; err != nil {
			return nil, err
		}
		parts := make([]string, 0, len(rows))
		for _, row := range rows {
			if row.Provider != PaymentProviderWaffoPancake {
				if row.Identity != nil && *row.Identity != "" {
					return nil, fmt.Errorf("%w: non-Waffo row has buyer identity in %s", ErrUnknownUserReference, table)
				}
				continue
			}
			if row.Identity == nil || *row.Identity == "" {
				return nil, fmt.Errorf("%w: empty historical Waffo identity in %s", ErrMigrationStateCorrupt, table)
			}
			if row.TradeNo == "" || !strings.HasPrefix(*row.Identity, "new-api-user-") {
				return nil, fmt.Errorf("%w: invalid historical Waffo row in %s", ErrMigrationStateCorrupt, table)
			}
			rowDigest := sha256.Sum256([]byte(strconv.FormatInt(row.ID, 10) + "\x00" + row.TradeNo + "\x00" + row.Provider + "\x00" + *row.Identity))
			parts = append(parts, strconv.FormatInt(row.ID, 10)+":"+hex.EncodeToString(rowDigest[:]))
		}
		digest := sha256.Sum256([]byte(strings.Join(parts, "\n")))
		result[table] = o023HistoricalWaffoSnapshot{Boundary: boundary, Count: int64(len(parts)), Digest: hex.EncodeToString(digest[:])}

		var newerRows []struct {
			Provider string
			Identity *string
		}
		if err := db.Table(table).Select("payment_provider AS provider, waffo_buyer_identity AS identity").Where("id > ? AND (payment_provider = ? OR waffo_buyer_identity IS NOT NULL AND waffo_buyer_identity <> '')", boundary, PaymentProviderWaffoPancake).Order("id").Scan(&newerRows).Error; err != nil {
			return nil, err
		}
		expectedPrefix := "wb_top_"
		if table == "subscription_orders" {
			expectedPrefix = "wb_sub_"
		}
		for _, row := range newerRows {
			if row.Provider != PaymentProviderWaffoPancake || row.Identity == nil || !strings.HasPrefix(*row.Identity, expectedPrefix) {
				return nil, fmt.Errorf("%w: invalid post-boundary Waffo row in %s", ErrMigrationStateCorrupt, table)
			}
		}
	}
	return result, nil
}

func o023WaffoBoundaries(snapshot o023InvariantSnapshot) map[string]int64 {
	boundaries := make(map[string]int64, len(snapshot.HistoricalWaffoDigest))
	for table, historical := range snapshot.HistoricalWaffoDigest {
		boundaries[table] = historical.Boundary
	}
	return boundaries
}

func o023AllowedHistoricalWaffoReport(snapshot o023InvariantSnapshot) map[string]int64 {
	report := map[string]int64{}
	for table, historical := range snapshot.HistoricalWaffoDigest {
		report["ALLOWED_HISTORICAL_WAFFO_SNAPSHOT:"+table] = historical.Count
	}
	return report
}

func o023EncodePersistedInvariant(snapshot o023InvariantSnapshot) (string, error) {
	digest, err := o023InvariantDigest(snapshot)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(o023PersistedInvariant{Version: O023InvariantContractVersion, Digest: digest, HistoricalWaffo: snapshot.HistoricalWaffoDigest})
	return string(encoded), err
}

func o023DecodePersistedInvariant(value string) (o023PersistedInvariant, error) {
	var persisted o023PersistedInvariant
	if err := json.Unmarshal([]byte(value), &persisted); err != nil || persisted.Version != O023InvariantContractVersion || persisted.Digest == "" || len(persisted.HistoricalWaffo) != 2 {
		return o023PersistedInvariant{}, ErrMigrationStateCorrupt
	}
	return persisted, nil
}

func o023CompareHistoricalWaffo(expected, actual map[string]o023HistoricalWaffoSnapshot) error {
	if len(expected) != len(actual) {
		return fmt.Errorf("%w: historical Waffo table set changed", ErrMigrationStateCorrupt)
	}
	for table, snapshot := range expected {
		if actual[table] != snapshot {
			return fmt.Errorf("%w: historical Waffo snapshot changed for %s", ErrMigrationStateCorrupt, table)
		}
	}
	return nil
}

func o023CanonicalStructuredValue(value string, mapping map[int]int) string {
	if strings.HasPrefix(value, "new-api-user-") {
		return value
	}
	value = o023RewriteDelimitedUserIDs(value, mapping)
	if parsed, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
		if next, ok := mapping[parsed]; ok {
			return strconv.Itoa(next)
		}
	}
	if strings.HasPrefix(value, "new-api-user-") {
		return value
	}
	return value
}

func o023RewriteDelimitedUserIDs(value string, mapping map[int]int) string {
	markers := []string{":user:", "user:"}
	for offset := 0; offset < len(value); {
		markerStart, marker := len(value), ""
		for _, candidate := range markers {
			if index := strings.Index(value[offset:], candidate); index >= 0 && offset+index < markerStart {
				markerStart, marker = offset+index, candidate
			}
		}
		if marker == "" {
			break
		}
		digitStart := markerStart + len(marker)
		digitEnd := digitStart
		for digitEnd < len(value) && value[digitEnd] >= '0' && value[digitEnd] <= '9' {
			digitEnd++
		}
		if digitEnd == digitStart {
			offset = digitStart
			continue
		}
		oldID, err := strconv.Atoi(value[digitStart:digitEnd])
		newID, ok := mapping[oldID]
		if err == nil && ok {
			value = value[:digitStart] + strconv.Itoa(newID) + value[digitEnd:]
			offset = digitStart + len(strconv.Itoa(newID))
			continue
		}
		offset = digitEnd
	}
	return value
}

func o023StructuredDigest(db *gorm.DB, mapping map[int]int) (string, error) {
	values := make([]string, 0)
	if exists, err := sqliteTableExists(db, "casbin_rule"); err != nil {
		return "", err
	} else if exists {
		var rows []struct{ V0 string }
		if err := db.Table("casbin_rule").Select("v0").Order("id").Scan(&rows).Error; err != nil {
			return "", err
		}
		for _, row := range rows {
			values = append(values, "casbin:"+o023CanonicalStructuredValue(row.V0, mapping))
		}
	}
	if exists, err := sqliteTableExists(db, "users"); err != nil {
		return "", err
	} else if exists {
		var rows []struct{ Value string }
		if err := db.Table("users").Select("setting AS value").Where("setting IS NOT NULL AND setting <> ''").Order("rowid").Scan(&rows).Error; err != nil {
			return "", err
		}
		for _, row := range rows {
			updated, err := o023JSONForDigest(row.Value, mapping)
			if err != nil {
				return "", fmt.Errorf("%w: structured digest users.setting: %v", ErrUnknownUserReference, err)
			}
			values = append(values, "users:setting:"+updated)
		}
	}
	for _, table := range []string{"payment_campaign_claims", "payment_campaign_participants", "top_ups", "subscription_orders"} {
		exists, err := sqliteTableExists(db, table)
		if err != nil {
			return "", err
		}
		if !exists {
			continue
		}
		for _, column := range []string{"claim_key", "email_claim_key", "participant_key", "email_participant_key", "campaign_slot_key", "waffo_buyer_identity"} {
			if ok, err := sqliteColumnExists(db, table, column); err != nil {
				return "", err
			} else if !ok {
				continue
			}
			var rows []struct{ Value string }
			if err := db.Table(table).Select(column + " AS value").Where(column + " IS NOT NULL AND " + column + " <> ''").Order("rowid").Scan(&rows).Error; err != nil {
				return "", err
			}
			for _, row := range rows {
				values = append(values, table+":"+column+":"+o023CanonicalStructuredValue(row.Value, mapping))
			}
		}
	}
	if exists, err := sqliteTableExists(db, "options"); err != nil {
		return "", err
	} else if exists {
		var option Option
		err := db.Where("key = ?", "payment_setting.compliance_confirmed_by").First(&option).Error
		if err == nil {
			values = append(values, "compliance:"+o023CanonicalStructuredValue(option.Value, mapping))
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return "", err
		}
	}
	for _, table := range []string{"tasks", "logs"} {
		exists, err := sqliteTableExists(db, table)
		if err != nil {
			return "", err
		}
		if !exists {
			continue
		}
		for _, column := range []string{"data", "private_data", "properties", "other"} {
			if ok, err := sqliteColumnExists(db, table, column); err != nil {
				return "", err
			} else if !ok {
				continue
			}
			var rows []struct{ Value string }
			if err := db.Table(table).Select(column + " AS value").Where(column + " IS NOT NULL AND " + column + " <> ''").Scan(&rows).Error; err != nil {
				return "", err
			}
			for _, row := range rows {
				updated, err := o023JSONForDigest(row.Value, mapping)
				if err != nil {
					return "", fmt.Errorf("%w: structured digest %s.%s: %v", ErrUnknownUserReference, table, column, err)
				}
				values = append(values, table+":"+column+":"+updated)
			}
		}
	}
	sort.Strings(values)
	digest := sha256.Sum256([]byte(strings.Join(values, "\n")))
	return hex.EncodeToString(digest[:]), nil
}

// Digest canonicalization may encounter either side of the migration. It
// must not treat an already-mapped new ID as an unknown legacy ID; strict
// validation is performed separately by the migration and residual verifier.
func o023JSONForDigest(raw string, mapping map[int]int) (string, error) {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return "", err
	}
	rewriteO023JSONValue(value, mapping)
	encoded, err := json.Marshal(value)
	return string(encoded), err
}

func o023CacheDigest(db *gorm.DB, mapping map[int]int) (string, error) {
	var users []int
	if err := db.Table("users").Order("id").Pluck("id", &users).Error; err != nil {
		return "", err
	}
	values := make([]string, 0, len(users)*5)
	for _, userID := range users {
		canonicalID := userID
		if next, ok := mapping[userID]; ok {
			canonicalID = next
		}
		values = append(values, getUserCacheKey(canonicalID), getUserAuthFenceKey(canonicalID), getUserAuthVersionKey(canonicalID))
		var tokens []string
		tokensExists, err := sqliteTableExists(db, "tokens")
		if err != nil {
			return "", err
		}
		if tokensExists {
			if err := db.Table("tokens").Where("user_id = ?", userID).Pluck("key", &tokens).Error; err != nil {
				return "", err
			}
			for _, key := range tokens {
				values = append(values, "token:"+common.GenerateHMAC(key))
			}
		}
		var sessions []string
		sessionsExists, err := sqliteTableExists(db, "user_sessions")
		if err != nil {
			return "", err
		}
		if sessionsExists {
			if err := db.Table("user_sessions").Where("user_id = ?", userID).Pluck("sid", &sessions).Error; err != nil {
				return "", err
			}
			for _, sid := range sessions {
				values = append(values, userSessionCacheKey(sid))
			}
		}
	}
	sort.Strings(values)
	digest := sha256.Sum256([]byte(strings.Join(values, "\n")))
	return hex.EncodeToString(digest[:]), nil
}

func o023InvariantDigest(snapshot o023InvariantSnapshot) (string, error) {
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func compareO023InvariantSnapshots(before, after o023InvariantSnapshot) error {
	if before.Users != after.Users || before.Quota != after.Quota || before.UsedQuota != after.UsedQuota || before.Requests != after.Requests {
		return fmt.Errorf("%w: user aggregate changed", ErrMigrationStateCorrupt)
	}
	if before.Facts == nil || len(before.Facts) != len(after.Facts) {
		return fmt.Errorf("%w: business table set changed", ErrMigrationStateCorrupt)
	}
	for key, value := range before.Facts {
		if after.Facts[key] != value {
			return fmt.Errorf("%w: aggregate changed for %s", ErrMigrationStateCorrupt, key)
		}
	}
	if before.Relations == nil || len(before.Relations) != len(after.Relations) {
		return fmt.Errorf("%w: relationship set changed", ErrMigrationStateCorrupt)
	}
	for key, value := range before.Relations {
		if after.Relations[key] != value {
			return fmt.Errorf("%w: relationship changed for %s", ErrMigrationStateCorrupt, key)
		}
	}
	if before.APIKeyCount != after.APIKeyCount || before.APIKeyDigest != after.APIKeyDigest {
		return fmt.Errorf("%w: API key summary changed", ErrMigrationStateCorrupt)
	}
	if before.SessionCount != after.SessionCount || after.ActiveSessions != 0 {
		return fmt.Errorf("%w: session invariant failed", ErrMigrationStateCorrupt)
	}
	if before.StructuredDigest != after.StructuredDigest || before.CacheDigest != after.CacheDigest {
		return fmt.Errorf("%w: structured or cache digest changed", ErrMigrationStateCorrupt)
	}
	if len(before.HistoricalWaffoDigest) != len(after.HistoricalWaffoDigest) {
		return fmt.Errorf("%w: historical Waffo snapshot table set changed", ErrMigrationStateCorrupt)
	}
	for table, digest := range before.HistoricalWaffoDigest {
		if after.HistoricalWaffoDigest[table] != digest {
			return fmt.Errorf("%w: historical Waffo snapshot changed for %s", ErrMigrationStateCorrupt, table)
		}
	}
	return nil
}

func verifyO023BusinessAggregates(db *gorm.DB) error {
	var invalid int64
	if err := db.Model(&User{}).Unscoped().Where("quota < 0 OR used_quota < 0 OR request_count < 0").Count(&invalid).Error; err != nil {
		return err
	}
	if invalid != 0 {
		return fmt.Errorf("negative user accounting aggregate: %d", invalid)
	}
	if exists, err := sqliteTableExists(db, "tokens"); err != nil {
		return err
	} else if exists {
		if err := db.Table("tokens").Where("key IS NULL OR length(CAST(key AS BLOB)) = 0 OR (remain_quota < 0 AND unlimited_quota = 0) OR used_quota < 0").Count(&invalid).Error; err != nil {
			return err
		}
		if invalid != 0 {
			return fmt.Errorf("invalid API key summary rows: %d", invalid)
		}
	}
	for _, table := range []string{"top_ups", "subscription_orders"} {
		exists, err := sqliteTableExists(db, table)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if err := db.Table(table).Where("money < 0").Count(&invalid).Error; err != nil {
			return err
		}
		if invalid != 0 {
			return fmt.Errorf("negative payment amount in %s: %d", table, invalid)
		}
	}
	if exists, err := sqliteTableExists(db, "user_subscriptions"); err != nil {
		return err
	} else if exists {
		if err := o023RequireNonNegativeColumn(db, "user_subscriptions", "amount_total"); err != nil {
			return err
		}
		if err := o023RequireNonNegativeColumn(db, "user_subscriptions", "amount_used"); err != nil {
			return err
		}
		if err := db.Table("user_subscriptions").Where("amount_total < 0 OR amount_used < 0 OR amount_used > amount_total").Count(&invalid).Error; err != nil {
			return err
		}
		if invalid != 0 {
			return fmt.Errorf("invalid subscription aggregate: %d", invalid)
		}
	}
	for _, pair := range []struct{ table, column string }{{"tasks", "quota"}, {"logs", "quota"}, {"checkins", "quota_awarded"}} {
		table, column := pair.table, pair.column
		exists, err := sqliteTableExists(db, table)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if err := o023RequireNonNegativeColumn(db, table, column); err != nil {
			return err
		}
		if err := db.Table(table).Where(column + " < 0").Count(&invalid).Error; err != nil {
			return err
		}
		if invalid != 0 {
			return fmt.Errorf("invalid quota aggregate in %s: %d", table, invalid)
		}
	}
	return nil
}

func o023RequireNonNegativeColumn(db *gorm.DB, table, column string) error {
	exists, err := sqliteColumnExists(db, table, column)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w: missing invariant column %s.%s", ErrMigrationStateCorrupt, table, column)
	}
	return nil
}

func o023APISummary(db *gorm.DB) (string, error) {
	exists, err := sqliteTableExists(db, "tokens")
	if err != nil {
		return "", err
	}
	if !exists {
		return "", nil
	}
	var digest string
	if err := o023APISummaryInto(db, &digest); err != nil {
		return "", err
	}
	return digest, nil
}

func o023APISummaryInto(db *gorm.DB, digest *string) error {
	var rows []struct {
		Key         string
		Status      string
		RemainQuota int
		UsedQuota   int
	}
	if err := db.Raw("SELECT key, status, remain_quota, used_quota FROM tokens ORDER BY key, status, remain_quota, used_quota").Scan(&rows).Error; err != nil {
		return err
	}
	parts := make([]string, 0, len(rows))
	for _, row := range rows {
		keyDigest := sha256.Sum256([]byte(row.Key))
		parts = append(parts, hex.EncodeToString(keyDigest[:])+"|"+row.Status+"|"+strconv.Itoa(row.RemainQuota)+"|"+strconv.Itoa(row.UsedQuota))
	}
	sort.Strings(parts)
	hash := sha256.Sum256([]byte(strconv.Itoa(len(rows)) + "\n" + strings.Join(parts, "\n")))
	*digest = hex.EncodeToString(hash[:])
	return nil
}

func verifyO023SQLiteIntegrity(db *gorm.DB) error {
	var foreignRows []struct{ Table, Rowid, Parent, Fkid string }
	if err := db.Raw("PRAGMA foreign_key_check").Scan(&foreignRows).Error; err != nil {
		return err
	}
	if len(foreignRows) != 0 {
		return fmt.Errorf("foreign_key_check failed: %d rows", len(foreignRows))
	}
	var integrity string
	if err := db.Raw("PRAGMA integrity_check").Scan(&integrity).Error; err != nil {
		return err
	}
	if strings.ToLower(strings.TrimSpace(integrity)) != "ok" {
		return fmt.Errorf("integrity_check failed: %s", integrity)
	}
	return nil
}

func collectO023CacheKeys(ctx context.Context, db *gorm.DB, userIDs []int) ([]string, error) {
	keys := make([]string, 0, len(userIDs)*3)
	for _, userID := range userIDs {
		keys = append(keys, getUserCacheKey(userID), getUserAuthFenceKey(userID), getUserAuthVersionKey(userID))
		tokensExists, err := sqliteTableExists(db, "tokens")
		if err != nil {
			return nil, err
		}
		var rows []struct{ Key string }
		if tokensExists {
			if err := db.Table("tokens").Select("key").Where("user_id = ?", userID).Scan(&rows).Error; err != nil {
				return nil, err
			}
			for _, row := range rows {
				keys = append(keys, "token:"+common.GenerateHMAC(row.Key))
			}
		}
		var sessions []string
		sessionsExists, err := sqliteTableExists(db, "user_sessions")
		if err != nil {
			return nil, err
		}
		if sessionsExists {
			if err := db.Table("user_sessions").Where("user_id = ?", userID).Pluck("sid", &sessions).Error; err != nil {
				return nil, err
			}
			for _, sid := range sessions {
				keys = append(keys, userSessionCacheKey(sid))
			}
		}
		if common.RedisEnabled && common.RDB != nil {
			iterator := common.RDB.Scan(ctx, 0, fmt.Sprintf("auth:user:*:%d", userID), 0).Iterator()
			for iterator.Next(ctx) {
				keys = append(keys, iterator.Val())
			}
			if iterator.Err() != nil {
				return nil, iterator.Err()
			}
			notifyIterator := common.RDB.Scan(ctx, 0, fmt.Sprintf("notify_limit:%d:*", userID), 0).Iterator()
			for notifyIterator.Next(ctx) {
				keys = append(keys, notifyIterator.Val())
			}
			if notifyIterator.Err() != nil {
				return nil, notifyIterator.Err()
			}
		}
	}
	return keys, nil
}

func invalidateO023Caches(ctx context.Context, cacheKeys []string) error {
	if !common.RedisEnabled || common.RDB == nil || len(cacheKeys) == 0 {
		return nil
	}
	unique := make(map[string]struct{}, len(cacheKeys))
	keys := make([]string, 0, len(cacheKeys))
	for _, key := range cacheKeys {
		if key == "" {
			continue
		}
		if _, exists := unique[key]; exists {
			continue
		}
		unique[key] = struct{}{}
		keys = append(keys, key)
	}
	return common.RDB.Del(ctx, keys...).Err()
}

func o023FillWaffoSnapshots(tx *gorm.DB) error {
	for _, table := range []string{"top_ups", "subscription_orders"} {
		exists, err := sqliteTableExists(tx, table)
		if err != nil || !exists {
			continue
		}
		column, err := sqliteColumnExists(tx, table, "waffo_buyer_identity")
		if err != nil {
			return err
		}
		if !column {
			return fmt.Errorf("missing Waffo snapshot column in %s", table)
		}
		if err := tx.Exec("UPDATE "+table+" SET waffo_buyer_identity = 'new-api-user-' || user_id WHERE payment_provider = ? AND (waffo_buyer_identity IS NULL OR waffo_buyer_identity = '')", PaymentProviderWaffoPancake).Error; err != nil {
			return err
		}
		var missing int64
		if err := tx.Table(table).Where("payment_provider = ? AND (waffo_buyer_identity IS NULL OR waffo_buyer_identity = '')", PaymentProviderWaffoPancake).Count(&missing).Error; err != nil {
			return err
		}
		if missing != 0 {
			return fmt.Errorf("missing Waffo snapshot rows in %s: %d", table, missing)
		}
	}
	return setO023Option(tx, O023WaffoSnapshotVersion, "1")
}

func MigrateO023UserIDs(db *gorm.DB) error {
	var callbackErr error
	connectionErr := db.Connection(func(conn *gorm.DB) error {
		callbackErr = migrateO023UserIDsOnConn(conn)
		return callbackErr
	})
	if callbackErr != nil {
		return callbackErr
	}
	return connectionErr
}

func migrateO023UserIDsOnConn(db *gorm.DB) (err error) {
	connPool := db.Statement.ConnPool
	db = db.Session(&gorm.Session{NewDB: true})
	db.Statement.ConnPool = connPool
	if err := verifyO023RedisTopology(context.Background()); err != nil {
		return err
	}
	var originalForeignKeys int
	if err = db.Raw("PRAGMA foreign_keys").Scan(&originalForeignKeys).Error; err != nil {
		return err
	}
	var originalDeferForeignKeys int
	if err = db.Raw("PRAGMA defer_foreign_keys").Scan(&originalDeferForeignKeys).Error; err != nil {
		return err
	}
	restored := false
	defer func() {
		if restored {
			return
		}
		value := "OFF"
		if originalForeignKeys == 1 {
			value = "ON"
		}
		if restoreErr := execO023Pragma(db, "PRAGMA foreign_keys = "+value); err == nil && restoreErr != nil {
			err = restoreErr
		}
		deferValue := "OFF"
		if originalDeferForeignKeys == 1 {
			deferValue = "ON"
		}
		if restoreErr := execO023Pragma(db, "PRAGMA defer_foreign_keys = "+deferValue); err == nil && restoreErr != nil {
			err = restoreErr
		}
	}()
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		return err
	}
	var foreignKeys int
	if err := db.Raw("PRAGMA foreign_keys").Scan(&foreignKeys).Error; err != nil || foreignKeys != 1 {
		return fmt.Errorf("foreign_keys pragma not enabled")
	}
	if err := AuditO023Schema(db); err != nil {
		return err
	}
	schemaLineage, err := o023SchemaLineageFor(db, true)
	if err != nil {
		return err
	}
	if exists, err := sqliteTableExists(db, "passkey_credentials"); err != nil {
		return err
	} else if exists {
		if system_setting.GetPasskeySettings().Enabled {
			return fmt.Errorf("DATA_RISK: Passkey must be disabled during O-023 migration")
		}
		var passkeyCount int64
		if err := db.Table("passkey_credentials").Count(&passkeyCount).Error; err != nil {
			return err
		}
		if passkeyCount != 0 {
			return fmt.Errorf("DATA_RISK: passkey credentials must be zero")
		}
	}
	status, err := O023MigrationStatus(db)
	if err != nil {
		return err
	}
	if status == "ALREADY_MIGRATED" {
		if err := VerifyO023Invariants(db); err != nil {
			return err
		}
		if err := verifyO023RawResiduals(db, nil); err != nil {
			return err
		}
		if err := verifyO023RedisHasNoLegacyUserIDKeys(context.Background()); err != nil {
			return err
		}
		storedValue, present, err := o023OptionValue(db, O023InvariantHash)
		if err != nil {
			return err
		}
		if !present {
			return fmt.Errorf("%w: invariant snapshot is missing", ErrMigrationStateCorrupt)
		}
		persisted, err := o023DecodePersistedInvariant(storedValue)
		if err != nil {
			return err
		}
		boundaries := map[string]int64{}
		for table, historical := range persisted.HistoricalWaffo {
			boundaries[table] = historical.Boundary
		}
		current, err := o023InvariantSnapshotForBoundaries(db, nil, boundaries)
		if err != nil {
			return err
		}
		if err := o023CompareHistoricalWaffo(persisted.HistoricalWaffo, current.HistoricalWaffoDigest); err != nil {
			return err
		}
		_ = o023AllowedHistoricalWaffoReport(current)
		return AuditO023Schema(db)
	}
	apiSummaryBefore, err := o023APISummary(db)
	if err != nil {
		return err
	}
	var cacheKeys []string
	if err := db.Exec("BEGIN IMMEDIATE").Error; err != nil {
		return err
	}
	tx := db.Session(&gorm.Session{SkipDefaultTransaction: true})
	committed := false
	defer func() {
		if !committed {
			_ = tx.Exec("ROLLBACK").Error
		}
	}()
	if err := tx.Exec("PRAGMA defer_foreign_keys = ON").Error; err != nil {
		return err
	}
	var deferred int
	if err := tx.Raw("PRAGMA defer_foreign_keys").Scan(&deferred).Error; err != nil || deferred != 1 {
		return fmt.Errorf("defer_foreign_keys pragma not enabled")
	}
	if err := tx.Exec("CREATE TEMP TABLE temp_o023_user_ids (old_id INTEGER PRIMARY KEY, new_id INTEGER NOT NULL UNIQUE)").Error; err != nil {
		return err
	}
	if err := invokeO023MigrationHook("after-temp"); err != nil {
		return err
	}
	var users []User
	if err := tx.Unscoped().Find(&users).Error; err != nil {
		return err
	}
	userIDs := make([]int, 0, len(users))
	mapping := make(map[int]int, len(users))
	for _, user := range users {
		userIDs = append(userIDs, user.Id)
		newID, err := randomUserID()
		if err != nil {
			return err
		}
		if err := tx.Exec("INSERT INTO temp_o023_user_ids(old_id, new_id) VALUES (?, ?)", user.Id, newID).Error; err != nil {
			return err
		}
		mapping[user.Id] = newID
	}
	cacheKeys, err = collectO023CacheKeys(context.Background(), db, userIDs)
	if err != nil {
		return err
	}
	if err := o023FillWaffoSnapshots(tx); err != nil {
		return err
	}
	invariantBefore, err := o023InvariantSnapshotFor(tx, mapping)
	if err != nil {
		return err
	}
	if err := migrateO023StructuredReferences(tx); err != nil {
		return err
	}
	if err := invokeO023MigrationHook("after-structured"); err != nil {
		return err
	}
	for table, columns := range o023DirectUserReferences {
		for _, column := range columns {
			if table == "users" && column == "inviter_id" {
				continue
			}
			if err := migrateO023References(tx, table, column); err != nil {
				return err
			}
		}
	}
	if err := tx.Exec("UPDATE users SET id = (SELECT new_id FROM temp_o023_user_ids WHERE old_id = users.id)").Error; err != nil {
		return err
	}
	if err := tx.Exec("UPDATE users AS target SET inviter_id = (SELECT map.new_id FROM temp_o023_user_ids AS map WHERE map.old_id = target.inviter_id) WHERE target.inviter_id != 0").Error; err != nil {
		return err
	}
	if err := invokeO023MigrationHook("after-users"); err != nil {
		return err
	}
	newUserIDs := make([]int, 0, len(mapping))
	for _, newID := range mapping {
		newUserIDs = append(newUserIDs, newID)
	}
	newCacheKeys, err := collectO023CacheKeys(context.Background(), db, newUserIDs)
	if err != nil {
		return err
	}
	cacheKeys = append(cacheKeys, newCacheKeys...)
	if exists, err := sqliteTableExists(tx, "user_sessions"); err != nil {
		return err
	} else if exists {
		if err := tx.Exec("UPDATE user_sessions SET status = 'revoked', revoked_at = strftime('%s','now') WHERE status != 'revoked'").Error; err != nil {
			return err
		}
	}
	// Keep the same canonicalization map for both sides: the pre-migration
	// snapshot normalizes legacy references, while the post-migration snapshot
	// is already canonical. This also makes the comparison stable for a rerun.
	invariantAfter, err := o023InvariantSnapshotForBoundaries(tx, mapping, o023WaffoBoundaries(invariantBefore))
	if err != nil {
		return err
	}
	if err := compareO023InvariantSnapshots(invariantBefore, invariantAfter); err != nil {
		return err
	}
	if err := VerifyO023Invariants(tx); err != nil {
		return err
	}
	// The comparison snapshot is canonicalized with the old->new map, while
	// the persisted digest must represent the post-migration database exactly.
	invariantPersisted, err := o023InvariantSnapshotForBoundaries(tx, nil, o023WaffoBoundaries(invariantBefore))
	if err != nil {
		return err
	}
	schemaHash, err := o023SchemaFingerprint(tx)
	if err != nil {
		return err
	}
	if err := setO023Option(tx, O023SchemaHash, o023PersistedSchemaValue(schemaLineage, schemaHash)); err != nil {
		return err
	}
	if err := setO023Option(tx, O023UserIDMigrationVersion, strconv.Itoa(1)); err != nil {
		return err
	}
	invariantValue, err := o023EncodePersistedInvariant(invariantPersisted)
	if err != nil {
		return err
	}
	if err := setO023Option(tx, O023InvariantHash, invariantValue); err != nil {
		return err
	}
	if err := tx.Exec("DROP TABLE temp_o023_user_ids").Error; err != nil {
		return err
	}
	if err := tx.Exec("COMMIT").Error; err != nil {
		return err
	}
	committed = true
	var deferredAfter int
	if err := db.Raw("PRAGMA defer_foreign_keys").Scan(&deferredAfter).Error; err != nil {
		return err
	}
	if deferredAfter != 0 {
		return fmt.Errorf("defer_foreign_keys was not cleared")
	}
	if err := VerifyO023Invariants(db); err != nil {
		return err
	}
	storedValue, present, err := o023OptionValue(db, O023InvariantHash)
	if err != nil {
		return err
	}
	if !present {
		return fmt.Errorf("%w: post-commit invariant snapshot is missing", ErrMigrationStateCorrupt)
	}
	persisted, err := o023DecodePersistedInvariant(storedValue)
	if err != nil {
		return err
	}
	boundaries := map[string]int64{}
	for table, historical := range persisted.HistoricalWaffo {
		boundaries[table] = historical.Boundary
	}
	current, err := o023InvariantSnapshotForBoundaries(db, nil, boundaries)
	if err != nil {
		return err
	}
	currentHash, err := o023InvariantDigest(current)
	if err != nil {
		return err
	}
	if persisted.Digest != currentHash {
		return fmt.Errorf("%w: post-commit invariant snapshot mismatch", ErrMigrationStateCorrupt)
	}
	if err := o023CompareHistoricalWaffo(persisted.HistoricalWaffo, current.HistoricalWaffoDigest); err != nil {
		return err
	}
	apiSummaryAfter, err := o023APISummary(db)
	if err != nil {
		return err
	}
	if apiSummaryBefore != apiSummaryAfter {
		return fmt.Errorf("API key summary changed during migration")
	}
	if err := invalidateO023Caches(context.Background(), cacheKeys); err != nil {
		return err
	}
	if err := verifyO023RawResiduals(db, userIDs); err != nil {
		return err
	}
	value := "OFF"
	if originalForeignKeys == 1 {
		value = "ON"
	}
	if err := db.Exec("PRAGMA foreign_keys = " + value).Error; err != nil {
		return err
	}
	var restoredValue int
	if err := db.Raw("PRAGMA foreign_keys").Scan(&restoredValue).Error; err != nil {
		return err
	}
	if restoredValue != originalForeignKeys {
		return fmt.Errorf("foreign_keys pragma restore failed")
	}
	restored = true
	return nil
}

func execO023Pragma(db *gorm.DB, query string) error {
	_, err := db.Statement.ConnPool.ExecContext(context.Background(), query)
	return err
}

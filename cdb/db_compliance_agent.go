package cdb

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
)

type (
	// CompAgentRuleset is a ruleset as the agents read it: each var a [name,
	// value] pair, or a [name, value, class] triplet.
	CompAgentRuleset struct {
		Name   string  `json:"name"`
		Filter string  `json:"filter"`
		Vars   [][]any `json:"vars"`
	}

	// CompAgentData is what the agent of a node needs for a compliance run, as
	// comp_get_data_v2 of the historical collector.
	CompAgentData struct {
		Modulesets          map[string][][2]any         `json:"modulesets"`
		Rulesets            map[string]CompAgentRuleset `json:"rulesets"`
		ModsetRsetRelations map[string][]string         `json:"modset_rset_relations"`
		ModsetRelations     map[string][]string         `json:"modset_relations"`
	}

	// CompAgentLog is the result of a module of a compliance run.
	CompAgentLog struct {
		Module  string
		Status  int
		Log     string
		Action  string
		RsetMD5 string
	}

	// compAgentTarget is what a compliance run applies to: a node, or an object
	// of that node when svcID is set. slave is the encapsulated instance of the
	// object.
	compAgentTarget struct {
		nodeID string
		svcID  string
		slave  bool
	}

	compRsetRelation struct {
		child  int
		rtype  string
		fsetID sql.NullInt64
	}

	compFsetRuleset struct {
		fsetID   int
		fsetName string
		rsetID   int
	}
)

// compAgentTeamCond restricts an auth_group alias to the groups publishing to
// the target: Everybody, the team responsible for the node, or those of the app
// of the object.
func compAgentTeamCond(alias string, t compAgentTarget) (string, []any) {
	if t.svcID != "" {
		return "(" + alias + ".role = 'Everybody' OR " + alias + `.id IN (
			SELECT apps_responsibles.group_id FROM services
			JOIN apps ON apps.app = services.svc_app
			JOIN apps_responsibles ON apps_responsibles.app_id = apps.id
			WHERE services.svc_id = ?))`, []any{t.svcID}
	}
	return "(" + alias + ".role = 'Everybody' OR " + alias + `.role = (
		SELECT team_responsible FROM nodes WHERE node_id = ?))`, []any{t.nodeID}
}

func (oDb *DB) compIntColumn(ctx context.Context, query string, args ...any) ([]int, error) {
	rows, err := oDb.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	l := make([]int, 0)
	for rows.Next() {
		var i int
		if err := rows.Scan(&i); err != nil {
			return nil, err
		}
		l = append(l, i)
	}
	return l, rows.Err()
}

// CompAgentNodeIsSlave tells whether the object runs in a container of the node.
func (oDb *DB) CompAgentNodeIsSlave(ctx context.Context, nodeID, svcID string) (bool, error) {
	var slave bool
	err := oDb.DB.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM svcmon JOIN nodes ON svcmon.mon_vmname = nodes.nodename
		WHERE nodes.node_id = ? AND svcmon.svc_id = ?)`, nodeID, svcID).Scan(&slave)
	return slave, err
}

// compAgentModulesetIDs returns the modulesets named, or those attached to the
// target when none is, that are published to the target.
func (oDb *DB) compAgentModulesetIDs(ctx context.Context, t compAgentTarget, names []string) ([]int, error) {
	var (
		join string
		args []any
	)
	cond, condArgs := compAgentTeamCond("auth_group", t)
	where := cond
	switch {
	case len(names) > 0:
		where += " AND comp_moduleset.modset_name IN (" + Placeholders(len(names)) + ")"
	case t.svcID != "":
		join = "JOIN comp_modulesets_services ON comp_modulesets_services.modset_id = comp_moduleset.id" +
			" AND comp_modulesets_services.svc_id = ? AND " + compSlaveCond("comp_modulesets_services.slave", t.slave)
		args = append(args, t.svcID)
	default:
		join = "JOIN comp_node_moduleset ON comp_node_moduleset.modset_id = comp_moduleset.id AND comp_node_moduleset.node_id = ?"
		args = append(args, t.nodeID)
	}
	args = append(args, condArgs...)
	for _, name := range names {
		args = append(args, name)
	}
	return oDb.compIntColumn(ctx, `SELECT DISTINCT comp_moduleset.id FROM comp_moduleset `+join+`
		JOIN comp_moduleset_team_publication ON comp_moduleset_team_publication.modset_id = comp_moduleset.id
		JOIN auth_group ON auth_group.id = comp_moduleset_team_publication.group_id
		WHERE `+where, args...)
}

func (oDb *DB) compIDNames(ctx context.Context, query string) (map[int]string, error) {
	rows, err := oDb.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	m := make(map[int]string)
	for rows.Next() {
		var (
			id   int
			name string
		)
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		m[id] = name
	}
	return m, rows.Err()
}

func (oDb *DB) compModsetChildren(ctx context.Context) (map[int][]int, error) {
	rows, err := oDb.DB.QueryContext(ctx, `SELECT parent_modset_id, child_modset_id FROM comp_moduleset_moduleset ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	m := make(map[int][]int)
	for rows.Next() {
		var parent, child int
		if err := rows.Scan(&parent, &child); err != nil {
			return nil, err
		}
		m[parent] = append(m[parent], child)
	}
	return m, rows.Err()
}

// compWithChildren returns ids and their descendants.
func compWithChildren(ids []int, children map[int][]int) []int {
	seen := make(map[int]bool)
	l := make([]int, 0, len(ids))
	var walk func(int)
	walk = func(id int) {
		if seen[id] {
			return
		}
		seen[id] = true
		l = append(l, id)
		for _, child := range children[id] {
			walk(child)
		}
	}
	for _, id := range ids {
		walk(id)
	}
	return l
}

func intArgs(l []int) []any {
	args := make([]any, len(l))
	for i, v := range l {
		args[i] = v
	}
	return args
}

func (oDb *DB) compAgentModulesets(ctx context.Context, ids []int) (map[string][][2]any, error) {
	m := make(map[string][][2]any)
	if len(ids) == 0 {
		return m, nil
	}
	rows, err := oDb.DB.QueryContext(ctx, `SELECT comp_moduleset.modset_name, comp_moduleset_modules.modset_mod_name,
		    comp_moduleset_modules.autofix IS NOT NULL AND comp_moduleset_modules.autofix IN ('T', '1')
		FROM comp_moduleset
		LEFT JOIN comp_moduleset_modules ON comp_moduleset_modules.modset_id = comp_moduleset.id
		WHERE comp_moduleset.id IN (`+Placeholders(len(ids))+`)
		ORDER BY comp_moduleset.id, comp_moduleset_modules.id`, intArgs(ids)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			modset  string
			module  sql.NullString
			autofix bool
		)
		if err := rows.Scan(&modset, &module, &autofix); err != nil {
			return nil, err
		}
		if _, ok := m[modset]; !ok {
			m[modset] = [][2]any{}
		}
		if module.Valid {
			m[modset] = append(m[modset], [2]any{module.String, autofix})
		}
	}
	return m, rows.Err()
}

func (oDb *DB) compNameRelations(ctx context.Context, query string) (map[string][]string, error) {
	rows, err := oDb.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	m := make(map[string][]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		m[key] = append(m[key], value)
	}
	return m, rows.Err()
}

// compAgentRow returns as vars the row a query selects, its columns not in
// blacklist named prefix.column. It returns nil without a row.
func (oDb *DB) compAgentRow(ctx context.Context, prefix string, blacklist []string, query string, args ...any) ([][]any, error) {
	rows, err := oDb.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, rows.Err()
	}
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return nil, err
	}
	l := make([][]any, 0, len(cols))
	for i, col := range cols {
		if slices.Contains(blacklist, col) {
			continue
		}
		l = append(l, []any{prefix + "." + col, compAgentValue(vals[i])})
	}
	return l, nil
}

func compAgentValue(v any) any {
	switch t := v.(type) {
	case []byte:
		return strings.Trim(string(t), "\x00")
	case string:
		return strings.Trim(t, "\x00")
	case time.Time:
		return t.Format(time.DateTime)
	default:
		return v
	}
}

func (oDb *DB) compAgentAssetRulesets(ctx context.Context, t compAgentTarget, rsets map[string]CompAgentRuleset) error {
	add := func(name string, vars [][]any) {
		if vars != nil {
			rsets[name] = CompAgentRuleset{Name: name, Filter: "", Vars: vars}
		}
	}
	vars, err := oDb.compAgentRow(ctx, "nodes", []string{"id"}, `SELECT * FROM nodes WHERE node_id = ?`, t.nodeID)
	if err != nil {
		return err
	}
	add("osvc_node", vars)
	if t.svcID == "" {
		return nil
	}
	vars, err = oDb.compAgentRow(ctx, "services", []string{"id", "svc_config"}, `SELECT * FROM services WHERE svc_id = ?`, t.svcID)
	if err != nil {
		return err
	}
	add("osvc_service", vars)
	blacklist := []string{"id", "mon_containerpath", "mon_vmem", "mon_vcpu"}
	const fresh = "svcmon.mon_updated > NOW() - INTERVAL 15 MINUTE"
	if t.slave {
		vars, err = oDb.compAgentRow(ctx, "svcmon", blacklist, `SELECT svcmon.* FROM svcmon
			JOIN nodes ON svcmon.mon_vmname = nodes.nodename
			WHERE svcmon.svc_id = ? AND nodes.node_id = ? AND svcmon.mon_containerstatus = 'up' AND `+fresh, t.svcID, t.nodeID)
	} else {
		vars, err = oDb.compAgentRow(ctx, "svcmon", blacklist, `SELECT svcmon.* FROM svcmon
			WHERE svcmon.svc_id = ? AND svcmon.node_id = ? AND `+fresh, t.svcID, t.nodeID)
	}
	if err != nil {
		return err
	}
	add("osvc_svcmon", vars)
	return nil
}

func (oDb *DB) compRsetRelations(ctx context.Context) (map[int][]compRsetRelation, error) {
	rows, err := oDb.DB.QueryContext(ctx, `SELECT comp_rulesets_rulesets.parent_rset_id, comp_rulesets_rulesets.child_rset_id,
		    comp_rulesets.ruleset_type, comp_rulesets_filtersets.fset_id
		FROM comp_rulesets_rulesets
		JOIN comp_rulesets ON comp_rulesets.id = comp_rulesets_rulesets.child_rset_id
		LEFT JOIN comp_rulesets_filtersets ON comp_rulesets_filtersets.ruleset_id = comp_rulesets.id
		ORDER BY comp_rulesets_rulesets.id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	m := make(map[int][]compRsetRelation)
	for rows.Next() {
		var (
			parent int
			r      compRsetRelation
		)
		if err := rows.Scan(&parent, &r.child, &r.rtype, &r.fsetID); err != nil {
			return nil, err
		}
		m[parent] = append(m[parent], r)
	}
	return m, rows.Err()
}

// compRsetDepths returns the depth of a ruleset and of its descendants: the
// explicit ones, and the contextual ones with a matching filterset. The
// descendants are not checked for ownership: the parent's applies.
func compRsetDepths(id int, relations map[int][]compRsetRelation, matching map[int]bool) map[int]int {
	depths := map[int]int{id: 0}
	var walk func(int, int)
	walk = func(parent, depth int) {
		for _, r := range relations[parent] {
			applies := r.rtype == "explicit" || (r.rtype == "contextual" && r.fsetID.Valid && matching[int(r.fsetID.Int64)])
			if !applies {
				continue
			}
			if _, seen := depths[r.child]; seen {
				continue
			}
			depths[r.child] = depth
			walk(r.child, depth+1)
		}
	}
	walk(id, 1)
	return depths
}

// compRulesetVars returns the vars of a ruleset and of its descendants, sorted
// by name. A var set at many depths takes the value of the deepest.
func (oDb *DB) compRulesetVars(ctx context.Context, depths map[int]int) ([][]any, error) {
	ids := make([]int, 0, len(depths))
	for id := range depths {
		ids = append(ids, id)
	}
	rows, err := oDb.DB.QueryContext(ctx, `SELECT ruleset_id, var_name, var_value, var_class
		FROM comp_rulesets_variables WHERE ruleset_id IN (`+Placeholders(len(ids))+`) ORDER BY id`, intArgs(ids)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	type candidate struct {
		depth int
		value []any
	}
	added := make(map[string]candidate)
	for rows.Next() {
		var (
			id                 int
			name, value, class sql.NullString
		)
		if err := rows.Scan(&id, &name, &value, &class); err != nil {
			return nil, err
		}
		if c, ok := added[name.String]; ok && depths[id] < c.depth {
			continue
		}
		added[name.String] = candidate{depth: depths[id], value: []any{name.String, value.String, class.String}}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(added))
	for name := range added {
		names = append(names, name)
	}
	slices.Sort(names)
	l := make([][]any, len(names))
	for i, name := range names {
		l[i] = added[name].value
	}
	return l, nil
}

func (oDb *DB) compFsetRulesets(ctx context.Context) ([]compFsetRuleset, error) {
	rows, err := oDb.DB.QueryContext(ctx, `SELECT DISTINCT comp_rulesets_filtersets.fset_id, gen_filtersets.fset_name, comp_rulesets.id
		FROM comp_rulesets_filtersets
		JOIN comp_rulesets ON comp_rulesets.id = comp_rulesets_filtersets.ruleset_id
		JOIN gen_filtersets ON gen_filtersets.id = comp_rulesets_filtersets.fset_id
		JOIN comp_rulesets_chains ON comp_rulesets_chains.tail_rset_id = comp_rulesets.id
		ORDER BY comp_rulesets_filtersets.fset_id, comp_rulesets.id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	l := make([]compFsetRuleset, 0)
	for rows.Next() {
		var r compFsetRuleset
		if err := rows.Scan(&r.fsetID, &r.fsetName, &r.rsetID); err != nil {
			return nil, err
		}
		l = append(l, r)
	}
	return l, rows.Err()
}

// compMatchingFsets returns the filtersets selecting the target. For an object
// they are resolved on the service, so a filter on the nodes matches when any
// node of the object does.
func (oDb *DB) compMatchingFsets(ctx context.Context, t compAgentTarget, l []compFsetRuleset) (map[int]bool, error) {
	field, id := "node_id", t.nodeID
	if t.svcID != "" {
		field, id = "svc_id", t.svcID
	}
	m := make(map[int]bool)
	seen := make(map[int]bool)
	for _, r := range l {
		if seen[r.fsetID] {
			continue
		}
		seen[r.fsetID] = true
		ids, err := oDb.ResolveFilterset(ctx, r.fsetID, field)
		if err != nil {
			return nil, fmt.Errorf("resolve filterset %s: %w", r.fsetName, err)
		}
		if slices.Contains(ids, id) {
			m[r.fsetID] = true
		}
	}
	return m, nil
}

func (oDb *DB) compAgentRulesets(ctx context.Context, t compAgentTarget, modsetIDs []int) (map[string]CompAgentRuleset, error) {
	rsets := make(map[string]CompAgentRuleset)
	if err := oDb.compAgentAssetRulesets(ctx, t, rsets); err != nil {
		return nil, err
	}
	relations, err := oDb.compRsetRelations(ctx)
	if err != nil {
		return nil, err
	}
	names, err := oDb.compIDNames(ctx, `SELECT id, ruleset_name FROM comp_rulesets`)
	if err != nil {
		return nil, err
	}
	fsetRulesets, err := oDb.compFsetRulesets(ctx)
	if err != nil {
		return nil, err
	}
	matching, err := oDb.compMatchingFsets(ctx, t, fsetRulesets)
	if err != nil {
		return nil, err
	}
	add := func(id int, filter string) error {
		name, ok := names[id]
		if !ok {
			return nil
		}
		vars, err := oDb.compRulesetVars(ctx, compRsetDepths(id, relations, matching))
		if err != nil {
			return err
		}
		rsets[name] = CompAgentRuleset{Name: name, Filter: filter, Vars: vars}
		return nil
	}

	public, err := oDb.compIntColumn(ctx, `SELECT id FROM comp_rulesets WHERE ruleset_public IN ('T', '1')`)
	if err != nil {
		return nil, err
	}
	viaModset := func(rtype string) ([]int, error) {
		if len(modsetIDs) == 0 {
			return nil, nil
		}
		return oDb.compIntColumn(ctx, `SELECT DISTINCT comp_moduleset_ruleset.ruleset_id FROM comp_moduleset_ruleset
			JOIN comp_rulesets ON comp_rulesets.id = comp_moduleset_ruleset.ruleset_id
			WHERE comp_rulesets.ruleset_type = ? AND comp_moduleset_ruleset.modset_id IN (`+Placeholders(len(modsetIDs))+`)
			ORDER BY comp_moduleset_ruleset.ruleset_id`, append([]any{rtype}, intArgs(modsetIDs)...)...)
	}

	contextual, err := viaModset("contextual")
	if err != nil {
		return nil, err
	}
	for _, r := range fsetRulesets {
		if !matching[r.fsetID] {
			continue
		}
		filter := r.fsetName
		if !slices.Contains(public, r.rsetID) {
			if !slices.Contains(contextual, r.rsetID) {
				continue
			}
			filter += " (matching non-public contextual ruleset shown via moduleset)"
		}
		if err := add(r.rsetID, filter); err != nil {
			return nil, err
		}
	}

	var attached []int
	if t.svcID != "" {
		attached, err = oDb.CompServiceRulesets(ctx, t.svcID, t.slave)
	} else {
		attached, err = oDb.CompNodeRulesets(ctx, t.nodeID)
	}
	if err != nil {
		return nil, err
	}
	for _, id := range attached {
		if err := add(id, "explicit attachment"); err != nil {
			return nil, err
		}
	}
	explicit, err := viaModset("explicit")
	if err != nil {
		return nil, err
	}
	for _, id := range explicit {
		if slices.Contains(attached, id) {
			continue
		}
		if err := add(id, "explicit attachment via moduleset"); err != nil {
			return nil, err
		}
	}
	return rsets, oDb.compAgentAddRunRuleset(ctx, rsets)
}

// compAgentAddRunRuleset stores the rulesets of the run, for the logs to refer
// to through their md5, and adds this md5 as the osvc_collector ruleset.
func (oDb *DB) compAgentAddRunRuleset(ctx context.Context, rsets map[string]CompAgentRuleset) error {
	b, err := json.Marshal(rsets)
	if err != nil {
		return err
	}
	sum := md5.Sum(b)
	rsetMD5 := hex.EncodeToString(sum[:])
	if _, err := oDb.DB.ExecContext(ctx, `INSERT INTO comp_run_ruleset (rset_md5, rset, date) VALUES (?, ?, NOW())
		ON DUPLICATE KEY UPDATE date = NOW()`, rsetMD5, b); err != nil {
		return err
	}
	rsets["osvc_collector"] = CompAgentRuleset{Name: "osvc_collector", Vars: [][]any{{"ruleset_md5", rsetMD5}}}
	return nil
}

// CompAgentData returns the data of a compliance run of a node, or of an object
// when svcID is set, on the modulesets named or else those attached.
func (oDb *DB) CompAgentData(ctx context.Context, nodeID, svcID string, slave bool, modulesets []string) (*CompAgentData, error) {
	t := compAgentTarget{nodeID: nodeID, svcID: svcID, slave: slave}
	names := slices.DeleteFunc(slices.Clone(modulesets), func(s string) bool { return s == "" })
	heads, err := oDb.compAgentModulesetIDs(ctx, t, names)
	if err != nil {
		return nil, fmt.Errorf("compAgentModulesetIDs: %w", err)
	}
	children, err := oDb.compModsetChildren(ctx)
	if err != nil {
		return nil, fmt.Errorf("compModsetChildren: %w", err)
	}
	ids := compWithChildren(heads, children)
	data := CompAgentData{}
	if data.Modulesets, err = oDb.compAgentModulesets(ctx, ids); err != nil {
		return nil, fmt.Errorf("compAgentModulesets: %w", err)
	}
	if data.Rulesets, err = oDb.compAgentRulesets(ctx, t, ids); err != nil {
		return nil, fmt.Errorf("compAgentRulesets: %w", err)
	}
	if data.ModsetRsetRelations, err = oDb.compNameRelations(ctx, `SELECT comp_moduleset.modset_name, comp_rulesets.ruleset_name
		FROM comp_moduleset_ruleset
		JOIN comp_moduleset ON comp_moduleset.id = comp_moduleset_ruleset.modset_id
		JOIN comp_rulesets ON comp_rulesets.id = comp_moduleset_ruleset.ruleset_id
		ORDER BY comp_moduleset_ruleset.id`); err != nil {
		return nil, fmt.Errorf("compNameRelations: %w", err)
	}
	if data.ModsetRelations, err = oDb.compNameRelations(ctx, `SELECT parent.modset_name, child.modset_name
		FROM comp_moduleset_moduleset
		JOIN comp_moduleset parent ON parent.id = comp_moduleset_moduleset.parent_modset_id
		JOIN comp_moduleset child ON child.id = comp_moduleset_moduleset.child_modset_id
		ORDER BY comp_moduleset_moduleset.id`); err != nil {
		return nil, fmt.Errorf("compNameRelations: %w", err)
	}
	return &data, nil
}

func (oDb *DB) compStringColumn(ctx context.Context, query string, args ...any) ([]string, error) {
	rows, err := oDb.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	l := make([]string, 0)
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		l = append(l, s)
	}
	return l, rows.Err()
}

// CompAgentModulesets returns the names of the modulesets published to a node.
func (oDb *DB) CompAgentModulesets(ctx context.Context, nodeID, pattern string) ([]string, error) {
	cond, args := compAgentTeamCond("auth_group", compAgentTarget{nodeID: nodeID})
	return oDb.compStringColumn(ctx, `SELECT DISTINCT comp_moduleset.modset_name FROM comp_moduleset
		JOIN comp_moduleset_team_publication ON comp_moduleset_team_publication.modset_id = comp_moduleset.id
		JOIN auth_group ON auth_group.id = comp_moduleset_team_publication.group_id
		WHERE comp_moduleset.modset_name LIKE ? AND `+cond+` ORDER BY comp_moduleset.modset_name`,
		append([]any{pattern}, args...)...)
}

// CompAgentRulesets returns the names of the public explicit rulesets published
// to a node.
func (oDb *DB) CompAgentRulesets(ctx context.Context, nodeID, pattern string) ([]string, error) {
	cond, args := compAgentTeamCond("auth_group", compAgentTarget{nodeID: nodeID})
	return oDb.compStringColumn(ctx, `SELECT DISTINCT comp_rulesets.ruleset_name FROM comp_rulesets
		JOIN comp_ruleset_team_publication ON comp_ruleset_team_publication.ruleset_id = comp_rulesets.id
		JOIN auth_group ON auth_group.id = comp_ruleset_team_publication.group_id
		WHERE comp_rulesets.ruleset_name LIKE ? AND comp_rulesets.ruleset_type = 'explicit'
		AND comp_rulesets.ruleset_public IN ('T', '1') AND `+cond+` ORDER BY comp_rulesets.ruleset_name`,
		append([]any{pattern}, args...)...)
}

// CompAgentModulesetID returns the id of the moduleset of that name, "" if none.
func (oDb *DB) CompAgentModulesetID(ctx context.Context, name string) (string, error) {
	return oDb.compIDByName(ctx, `SELECT id FROM comp_moduleset WHERE modset_name = ?`, name)
}

// CompAgentRulesetID returns the id of the ruleset of that name, "" if none.
func (oDb *DB) CompAgentRulesetID(ctx context.Context, name string) (string, error) {
	return oDb.compIDByName(ctx, `SELECT id FROM comp_rulesets WHERE ruleset_name = ?`, name)
}

func (oDb *DB) compIDByName(ctx context.Context, query, name string) (string, error) {
	var id string
	switch err := oDb.DB.QueryRowContext(ctx, query, name).Scan(&id); err {
	case nil:
		return id, nil
	case sql.ErrNoRows:
		return "", nil
	default:
		return "", err
	}
}

// compAgentCleanLog keeps what the utf8 columns of the logs store: no control
// character but the line and tab ones, and no character above the BMP.
func compAgentCleanLog(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r > 0xFFFF || r == unicode.ReplacementChar || unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.ToValidUTF8(s, ""))
}

// CompAgentInsertLogs records the results of a compliance run: in comp_log, and
// for a check in comp_status and comp_log_daily.
func (oDb *DB) CompAgentInsertLogs(ctx context.Context, nodeID, svcID string, logs []CompAgentLog) error {
	for _, l := range logs {
		text := compAgentCleanLog(l.Log)
		if _, err := oDb.DB.ExecContext(ctx, `INSERT INTO comp_log
			    (run_module, run_status, run_log, run_date, run_action, rset_md5, node_id, svc_id)
			VALUES (?, ?, ?, NOW(), ?, ?, ?, ?)`, l.Module, l.Status, text, l.Action, l.RsetMD5, nodeID, svcID); err != nil {
			return fmt.Errorf("insert comp_log: %w", err)
		}
		if l.Action != "check" {
			continue
		}
		if _, err := oDb.DB.ExecContext(ctx, `INSERT INTO comp_status
			    (run_module, run_status, run_log, run_date, run_action, rset_md5, node_id, svc_id)
			VALUES (?, ?, ?, NOW(), ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE run_status = VALUES(run_status), run_log = VALUES(run_log),
			    run_date = VALUES(run_date), run_action = VALUES(run_action), rset_md5 = VALUES(rset_md5)`,
			l.Module, l.Status, text, l.Action, l.RsetMD5, nodeID, svcID); err != nil {
			return fmt.Errorf("insert comp_status: %w", err)
		}
		if _, err := oDb.DB.ExecContext(ctx, `INSERT INTO comp_log_daily (run_module, run_status, run_date, node_id, svc_id)
			VALUES (?, ?, CURDATE(), ?, ?)
			ON DUPLICATE KEY UPDATE run_status = VALUES(run_status)`, l.Module, l.Status, nodeID, svcID); err != nil {
			return fmt.Errorf("insert comp_log_daily: %w", err)
		}
	}
	return nil
}

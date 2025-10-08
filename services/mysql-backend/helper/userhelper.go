package helper

import "strings"

// escapeSQLString 简单转义用于单引号包裹的 MySQL 字符串字面量
func EscapeSQLString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	return s
}

// UniqueStrings returns a new slice with duplicates removed, preserving the first-seen order.
func UniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// ParseDatabasesFromGrants parses SHOW GRANTS lines and extracts database names the user has privileges on.
// Rules:
// - "ON *.*" => returns "*" indicating global privileges.
// - "ON `db`.*" or "ON db.*" => returns "db".
// - "ON `db`.`table`" or "ON db.table" => returns "db".
// It preserves discovery order and de-duplicates results.
func ParseDatabasesFromGrants(grants []string) []string {
	if len(grants) == 0 {
		return nil
	}

	out := make([]string, 0, len(grants))
	seen := make(map[string]struct{}, len(grants))

	add := func(db string) {
		db = strings.TrimSpace(db)
		if db == "" {
			return
		}
		if _, ok := seen[db]; ok {
			return
		}
		seen[db] = struct{}{}
		out = append(out, db)
	}

	for _, g := range grants {
		s := strings.TrimSpace(g)
		if s == "" {
			continue
		}

		lower := strings.ToLower(s)
		idx := strings.Index(lower, " on ")
		if idx == -1 {
			continue
		}

		onPart := s[idx+4:]

		// Trim anything after " TO "
		if j := strings.Index(strings.ToLower(onPart), " to "); j != -1 {
			onPart = onPart[:j]
		}
		onPart = strings.TrimSpace(onPart)

		// Global privileges
		if onPart == "*.*" {
			add("*")
			continue
		}

		// Backticked patterns: `db`.* or `db`.`table`
		if strings.HasPrefix(onPart, "`") {
			rest := onPart[1:]
			// `db`.*  -> look for "`.*"
			if k := strings.Index(rest, "`.*"); k != -1 {
				add(rest[:k])
				continue
			}
			// `db`.`table` -> look for "`.`"
			if k := strings.Index(rest, "`.`"); k != -1 {
				add(rest[:k])
				continue
			}
		}

		// Non-backticked patterns: db.* or db.table
		if dot := strings.Index(onPart, "."); dot != -1 {
			add(onPart[:dot])
			continue
		}
	}

	return out
}

// ParsePrivilegesFromGrants parses SHOW GRANTS lines and extracts individual privilege names.
// Converts "GRANT SELECT, INSERT ON *.* TO 'user'@'host'" to ["SELECT", "INSERT"]
func ParsePrivilegesFromGrants(grants []string) []string {
	if len(grants) == 0 {
		return nil
	}

	var allPrivileges []string
	seen := make(map[string]struct{})

	for _, grant := range grants {
		grant = strings.TrimSpace(grant)
		if grant == "" {
			continue
		}

		// Find "GRANT" and "ON" positions
		lower := strings.ToLower(grant)
		grantIdx := strings.Index(lower, "grant ")
		onIdx := strings.Index(lower, " on ")

		if grantIdx == -1 || onIdx == -1 || grantIdx >= onIdx {
			continue
		}

		// Extract the privileges part between "GRANT" and "ON"
		privPart := grant[grantIdx+6 : onIdx] // +6 for "grant "
		privPart = strings.TrimSpace(privPart)

		// Split by comma and clean up each privilege
		privs := strings.Split(privPart, ",")
		for _, priv := range privs {
			priv = strings.TrimSpace(priv)
			if priv == "" {
				continue
			}

			// Convert to uppercase for consistency
			priv = strings.ToUpper(priv)

			// Skip if already seen
			if _, exists := seen[priv]; exists {
				continue
			}

			seen[priv] = struct{}{}
			allPrivileges = append(allPrivileges, priv)
		}
	}

	return allPrivileges
}

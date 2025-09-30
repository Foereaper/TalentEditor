// Copyright (c) 2025 TalentEditor
//
// TalentEditor is licensed under the MIT License.
// See the LICENSE file for details.

package main

import (
    "database/sql"
    "fmt"
    "strings"
)

// --- TalentTab queries ---
func GetAllTalentTabs(ctx *AppContext) (map[int]TalentTab, error) {
    query := `
        SELECT id, name_enus, spell_icon, class_mask, order_index, background_file, creature_family
        FROM TalentTab
        ORDER BY id`
    
    rows, err := queryWithDebug(ctx.DB, query)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    tabs := make(map[int]TalentTab)
    for rows.Next() {
        var t TalentTab
        var name sql.NullString
        if err := rows.Scan(&t.ID, &name, &t.SpellIcon, &t.ClassMask, &t.OrderIndex, &t.Background, &t.CreatureFamily,); err != nil {
            return nil, err
        }

        if name.Valid {
            t.NameENUS = name.String
        } else {
            t.NameENUS = fmt.Sprintf("tab_%d", t.ID)
        }

        tabs[t.ID] = t
    }

    if err := rows.Err(); err != nil {
        return nil, err
    }

    return tabs, nil
}

// --- Spell queries ---
func GetSpellsByIDs(ctx *AppContext, ids []int) (map[int]Spell, error) {
    result := make(map[int]Spell)
    if len(ids) == 0 {
        return result, nil
    }

    // Initialize cache if nil
    if ctx.Spells == nil {
        ctx.Spells = make(map[int]Spell)
    }

    // Split requested IDs into cached vs. missing
    var missing []int
    for _, id := range ids {
        if spell, ok := ctx.Spells[id]; ok {
            result[id] = spell
        } else {
            missing = append(missing, id)
        }
    }

    // If nothing missing, we’re done
    if len(missing) == 0 {
        return result, nil
    }

    // Build SQL placeholders for missing IDs
    placeholders := make([]string, len(missing))
    args := make([]interface{}, len(missing))
    for i, id := range missing {
        placeholders[i] = "?"
        args[i] = id
    }

    query := fmt.Sprintf(`
        SELECT id, spell_name_enus, spell_icon_id, spell_desc_enus
        FROM Spell
        WHERE id IN (%s)`, strings.Join(placeholders, ","))

    rows, err := queryWithDebug(ctx.DB, query, args...)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    for rows.Next() {
        var s Spell
        if err := rows.Scan(&s.ID, &s.NameENUS, &s.IconID, &s.Desc); err != nil {
            return nil, err
        }

        // Update cache and result
        ctx.Spells[s.ID] = s
        result[s.ID] = s
    }

    return result, nil
}

func GetTalentsForSpec(ctx *AppContext, specID int) ([]Talent, []int, error) {
    query := `
        SELECT id, spec_id, tier_id, column_index,
               rank_1, rank_2, rank_3, rank_4, rank_5, rank_6, rank_7, rank_8, rank_9,
               pre_req_talent_1, pre_req_talent_2, pre_req_talent_3,
               pre_req_rank_1, pre_req_rank_2, pre_req_rank_3,
               flags, req_spell_id, allow_for_pet_flags_1, allow_for_pet_flags_2
        FROM Talent
        WHERE spec_id = ?`

    rows, err := queryWithDebug(ctx.DB, query, specID)
    if err != nil {
        return nil, nil, err
    }
    defer rows.Close()

    var talents []Talent
    spellIDMap := make(map[int]struct{})

    for rows.Next() {
        var t Talent
        var tier, col sql.NullInt64

        if err := rows.Scan(
            &t.ID, &t.SpecID, &tier, &col,
            &t.Rank[0], &t.Rank[1], &t.Rank[2], &t.Rank[3], &t.Rank[4],
            &t.Rank[5], &t.Rank[6], &t.Rank[7], &t.Rank[8],
            &t.PreReqTalent[0], &t.PreReqTalent[1], &t.PreReqTalent[2],
            &t.PreReqRank[0], &t.PreReqRank[1], &t.PreReqRank[2],
            &t.Flags, &t.ReqSpellID, &t.AllowForPetFlags1, &t.AllowForPetFlags2,
        ); err != nil {
            return nil, nil, err
        }

        t.TierID = tier
        t.ColumnIndex = col
        talents = append(talents, t)

        // Collect first-rank spell IDs
        if t.Rank[0].Valid {
            spellIDMap[int(t.Rank[0].Int64)] = struct{}{}
        }
    }

    // Convert map keys to slice
    spellIDs := make([]int, 0, len(spellIDMap))
    for id := range spellIDMap {
        spellIDs = append(spellIDs, id)
    }

    return talents, spellIDs, nil
}

// --- SpellIcon queries ---
func GetAllSpellIcons(ctx *AppContext) (map[int]string, error) {
    if ctx.SpellIcons != nil {
        return ctx.SpellIcons, nil
    }

    rows, err := queryWithDebug(ctx.DB, "SELECT id, name FROM SpellIcon")
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    
    trimPrefixCaseInsensitive := func(s, prefix string) string {
        if len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix) {
            return s[len(prefix):]
        }
        return s
    }
    
    icons := make(map[int]string)
    for rows.Next() {
        var id int
        var name sql.NullString
        if err := rows.Scan(&id, &name); err != nil {
            return nil, err
        }
        if name.Valid {
            icons[id] = trimPrefixCaseInsensitive(name.String, `Interface\Icons\`)
        }
    }
    
    ctx.SpellIcons = icons
    return icons, nil
}

// --- Classes queries ---
func GetAllClasses(ctx *AppContext) (map[int]ChrClass, error) {
    rows, err := queryWithDebug(ctx.DB, "SELECT id, name_enus, pet_name_token FROM ChrClasses")
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    classes := make(map[int]ChrClass)
    for rows.Next() {
        var c ChrClass
        if err := rows.Scan(&c.ID, &c.NameENUS, &c.PetName); err != nil {
            return nil, err
        }
        classes[c.ID] = c
    }

    // Check for iteration errors
    if err := rows.Err(); err != nil {
        return nil, err
    }

    return classes, nil
}


// --- Insert/Update/Delete Talent ---
func InsertTalentQuery(t *Talent) (string, []interface{}) {
    query := `INSERT INTO Talent (
        id, spec_id, tier_id, column_index,
        rank_1, rank_2, rank_3, rank_4, rank_5, rank_6, rank_7, rank_8, rank_9,
        pre_req_talent_1, pre_req_talent_2, pre_req_talent_3,
        pre_req_rank_1, pre_req_rank_2, pre_req_rank_3,
        flags, req_spell_id, allow_for_pet_flags_1, allow_for_pet_flags_2
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
    args := []interface{}{
        t.ID,
        nullInt64ToInterface(t.SpecID), nullInt64ToInterface(t.TierID), nullInt64ToInterface(t.ColumnIndex),
        nullInt64ToInterface(t.Rank[0]), nullInt64ToInterface(t.Rank[1]), nullInt64ToInterface(t.Rank[2]),
        nullInt64ToInterface(t.Rank[3]), nullInt64ToInterface(t.Rank[4]), nullInt64ToInterface(t.Rank[5]),
        nullInt64ToInterface(t.Rank[6]), nullInt64ToInterface(t.Rank[7]), nullInt64ToInterface(t.Rank[8]),
        nullInt64ToInterface(t.PreReqTalent[0]), nullInt64ToInterface(t.PreReqTalent[1]), nullInt64ToInterface(t.PreReqTalent[2]),
        nullInt64ToInterface(t.PreReqRank[0]), nullInt64ToInterface(t.PreReqRank[1]), nullInt64ToInterface(t.PreReqRank[2]),
        nullInt64ToInterface(t.Flags), nullInt64ToInterface(t.ReqSpellID),
        nullInt64ToInterface(t.AllowForPetFlags1), nullInt64ToInterface(t.AllowForPetFlags2),
    }
    return query, args
}

func UpdateTalentQuery(t *Talent) (string, []interface{}) {
    query := `UPDATE Talent SET
        spec_id=?, tier_id=?, column_index=?,
        rank_1=?, rank_2=?, rank_3=?, rank_4=?, rank_5=?, rank_6=?, rank_7=?, rank_8=?, rank_9=?,
        pre_req_talent_1=?, pre_req_talent_2=?, pre_req_talent_3=?,
        pre_req_rank_1=?, pre_req_rank_2=?, pre_req_rank_3=?,
        flags=?, req_spell_id=?, allow_for_pet_flags_1=?, allow_for_pet_flags_2=?
        WHERE id=?`
    args := []interface{}{
        nullInt64ToInterface(t.SpecID), nullInt64ToInterface(t.TierID), nullInt64ToInterface(t.ColumnIndex),
        nullInt64ToInterface(t.Rank[0]), nullInt64ToInterface(t.Rank[1]), nullInt64ToInterface(t.Rank[2]),
        nullInt64ToInterface(t.Rank[3]), nullInt64ToInterface(t.Rank[4]), nullInt64ToInterface(t.Rank[5]),
        nullInt64ToInterface(t.Rank[6]), nullInt64ToInterface(t.Rank[7]), nullInt64ToInterface(t.Rank[8]),
        nullInt64ToInterface(t.PreReqTalent[0]), nullInt64ToInterface(t.PreReqTalent[1]), nullInt64ToInterface(t.PreReqTalent[2]),
        nullInt64ToInterface(t.PreReqRank[0]), nullInt64ToInterface(t.PreReqRank[1]), nullInt64ToInterface(t.PreReqRank[2]),
        nullInt64ToInterface(t.Flags), nullInt64ToInterface(t.ReqSpellID),
        nullInt64ToInterface(t.AllowForPetFlags1), nullInt64ToInterface(t.AllowForPetFlags2),
        t.ID,
    }
    return query, args
}

func DeleteTalentQuery(id int) (string, []interface{}) {
    return "DELETE FROM Talent WHERE id = ?", []interface{}{id}
}

// Query with automatic error logging
func queryWithDebug(db *sql.DB, query string, args ...interface{}) (*sql.Rows, error) {
    rows, err := db.Query(query, args...)
    if err != nil {
        fmt.Printf("[SQL Error]\nQuery: %s\nArgs: %v\nError: %v\n", query, args, err)
    }
    return rows, err
}

// Exec with automatic error logging
func execWithDebug(db *sql.DB, query string, args ...interface{}) (sql.Result, error) {
    res, err := db.Exec(query, args...)
    if err != nil {
        fmt.Printf("[SQL Exec Error]\nQuery: %s\nArgs: %v\nError: %v\n", query, args, err)
    }
    return res, err
}

func nullInt64ToInterface(n sql.NullInt64) interface{} {
	if n.Valid {
		return n.Int64
	}
	return nil
}
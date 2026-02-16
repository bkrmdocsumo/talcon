package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/user/talon/internal/config"
)

// ─── Plugin Data Model ───

type PluginCommand struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   int64  `json:"createdAt"`
	UpdatedAt   int64  `json:"updatedAt"`
}

type PluginSkill struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   int64  `json:"createdAt"`
	UpdatedAt   int64  `json:"updatedAt"`
}

type PluginsData struct {
	Commands []PluginCommand `json:"commands"`
	Skills   []PluginSkill   `json:"skills"`
}

type PluginCommandDetail struct {
	PluginCommand
	Body string `json:"body"`
}

type PluginSkillDetail struct {
	PluginSkill
	Body string `json:"body"`
}

var pluginsMu sync.Mutex

func pluginsJSONPath() (string, error) {
	baseDir, err := config.TalonDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, "plugins.json"), nil
}

func pluginsDir(kind string) (string, error) {
	baseDir, err := config.TalonDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, "plugins", kind), nil
}

func readPluginsData() (PluginsData, error) {
	path, err := pluginsJSONPath()
	if err != nil {
		return PluginsData{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return PluginsData{Commands: []PluginCommand{}, Skills: []PluginSkill{}}, nil
		}
		return PluginsData{}, fmt.Errorf("read plugins.json: %w", err)
	}
	if len(data) == 0 {
		return PluginsData{Commands: []PluginCommand{}, Skills: []PluginSkill{}}, nil
	}
	var pd PluginsData
	if err := json.Unmarshal(data, &pd); err != nil {
		return PluginsData{}, fmt.Errorf("parse plugins.json: %w", err)
	}
	if pd.Commands == nil {
		pd.Commands = []PluginCommand{}
	}
	if pd.Skills == nil {
		pd.Skills = []PluginSkill{}
	}
	return pd, nil
}

func writePluginsData(pd PluginsData) error {
	path, err := pluginsJSONPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(pd, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal plugins.json: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write plugins.json: %w", err)
	}
	return nil
}

func readMdFile(kind, name string) (string, error) {
	dir, err := pluginsDir(kind)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, name+".md"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read %s/%s.md: %w", kind, name, err)
	}
	return string(data), nil
}

func writeMdFile(kind, name, body string) error {
	dir, err := pluginsDir(kind)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	return os.WriteFile(filepath.Join(dir, name+".md"), []byte(body), 0o644)
}

func deleteMdFile(kind, name string) error {
	dir, err := pluginsDir(kind)
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dir, name+".md")); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func sanitizeName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "-")
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	result := b.String()
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}
	result = strings.Trim(result, "-")
	if result == "" {
		result = "untitled"
	}
	return result
}

// ─── Command CRUD ───

func (a *App) ListCommands() ([]PluginCommand, error) {
	pluginsMu.Lock()
	defer pluginsMu.Unlock()
	pd, err := readPluginsData()
	if err != nil {
		return nil, err
	}
	return pd.Commands, nil
}

func (a *App) GetCommand(id string) (PluginCommandDetail, error) {
	pluginsMu.Lock()
	defer pluginsMu.Unlock()
	pd, err := readPluginsData()
	if err != nil {
		return PluginCommandDetail{}, err
	}
	for _, cmd := range pd.Commands {
		if cmd.ID == id {
			body, err := readMdFile("commands", cmd.Name)
			if err != nil {
				return PluginCommandDetail{}, err
			}
			return PluginCommandDetail{PluginCommand: cmd, Body: body}, nil
		}
	}
	return PluginCommandDetail{}, fmt.Errorf("command %s not found", id)
}

func (a *App) AddCommand(name string, description string, body string) (PluginCommand, error) {
	name = sanitizeName(name)
	description = strings.TrimSpace(description)
	if name == "" {
		return PluginCommand{}, fmt.Errorf("command name cannot be empty")
	}
	pluginsMu.Lock()
	defer pluginsMu.Unlock()
	pd, err := readPluginsData()
	if err != nil {
		return PluginCommand{}, err
	}
	for _, cmd := range pd.Commands {
		if cmd.Name == name {
			return PluginCommand{}, fmt.Errorf("command %q already exists", name)
		}
	}
	now := time.Now().UnixMilli()
	cmd := PluginCommand{
		ID: fmt.Sprintf("cmd-%d", now), Name: name, Description: description,
		CreatedAt: now, UpdatedAt: now,
	}
	pd.Commands = append(pd.Commands, cmd)
	if err := writePluginsData(pd); err != nil {
		return PluginCommand{}, err
	}
	if err := writeMdFile("commands", name, body); err != nil {
		return PluginCommand{}, err
	}
	log.Printf("[plugins] added command: %s", name)
	return cmd, nil
}

func (a *App) UpdateCommand(id string, name string, description string, body string) error {
	name = sanitizeName(name)
	description = strings.TrimSpace(description)
	if name == "" {
		return fmt.Errorf("command name cannot be empty")
	}
	pluginsMu.Lock()
	defer pluginsMu.Unlock()
	pd, err := readPluginsData()
	if err != nil {
		return err
	}
	found := false
	var oldName string
	for i, cmd := range pd.Commands {
		if cmd.ID == id {
			oldName = cmd.Name
			if name != oldName {
				for _, other := range pd.Commands {
					if other.ID != id && other.Name == name {
						return fmt.Errorf("command %q already exists", name)
					}
				}
			}
			pd.Commands[i].Name = name
			pd.Commands[i].Description = description
			pd.Commands[i].UpdatedAt = time.Now().UnixMilli()
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("command %s not found", id)
	}
	if err := writePluginsData(pd); err != nil {
		return err
	}
	if oldName != name {
		_ = deleteMdFile("commands", oldName)
	}
	if err := writeMdFile("commands", name, body); err != nil {
		return err
	}
	log.Printf("[plugins] updated command %s: %s", id, name)
	return nil
}

func (a *App) DeleteCommand(id string) error {
	pluginsMu.Lock()
	defer pluginsMu.Unlock()
	pd, err := readPluginsData()
	if err != nil {
		return err
	}
	filtered := make([]PluginCommand, 0, len(pd.Commands))
	var deletedName string
	for _, cmd := range pd.Commands {
		if cmd.ID == id {
			deletedName = cmd.Name
			continue
		}
		filtered = append(filtered, cmd)
	}
	if deletedName == "" {
		return fmt.Errorf("command %s not found", id)
	}
	pd.Commands = filtered
	if err := writePluginsData(pd); err != nil {
		return err
	}
	_ = deleteMdFile("commands", deletedName)
	log.Printf("[plugins] deleted command %s (%s)", id, deletedName)
	return nil
}

// ─── Skill CRUD ───

func (a *App) ListSkills() ([]PluginSkill, error) {
	pluginsMu.Lock()
	defer pluginsMu.Unlock()
	pd, err := readPluginsData()
	if err != nil {
		return nil, err
	}
	return pd.Skills, nil
}

func (a *App) GetSkill(id string) (PluginSkillDetail, error) {
	pluginsMu.Lock()
	defer pluginsMu.Unlock()
	pd, err := readPluginsData()
	if err != nil {
		return PluginSkillDetail{}, err
	}
	for _, sk := range pd.Skills {
		if sk.ID == id {
			body, err := readMdFile("skills", sk.Name)
			if err != nil {
				return PluginSkillDetail{}, err
			}
			return PluginSkillDetail{PluginSkill: sk, Body: body}, nil
		}
	}
	return PluginSkillDetail{}, fmt.Errorf("skill %s not found", id)
}

func (a *App) AddSkill(name string, description string, body string) (PluginSkill, error) {
	name = sanitizeName(name)
	description = strings.TrimSpace(description)
	if name == "" {
		return PluginSkill{}, fmt.Errorf("skill name cannot be empty")
	}
	pluginsMu.Lock()
	defer pluginsMu.Unlock()
	pd, err := readPluginsData()
	if err != nil {
		return PluginSkill{}, err
	}
	for _, sk := range pd.Skills {
		if sk.Name == name {
			return PluginSkill{}, fmt.Errorf("skill %q already exists", name)
		}
	}
	now := time.Now().UnixMilli()
	sk := PluginSkill{
		ID: fmt.Sprintf("skill-%d", now), Name: name, Description: description,
		CreatedAt: now, UpdatedAt: now,
	}
	pd.Skills = append(pd.Skills, sk)
	if err := writePluginsData(pd); err != nil {
		return PluginSkill{}, err
	}
	if err := writeMdFile("skills", name, body); err != nil {
		return PluginSkill{}, err
	}
	log.Printf("[plugins] added skill: %s", name)
	return sk, nil
}

func (a *App) UpdateSkill(id string, name string, description string, body string) error {
	name = sanitizeName(name)
	description = strings.TrimSpace(description)
	if name == "" {
		return fmt.Errorf("skill name cannot be empty")
	}
	pluginsMu.Lock()
	defer pluginsMu.Unlock()
	pd, err := readPluginsData()
	if err != nil {
		return err
	}
	found := false
	var oldName string
	for i, sk := range pd.Skills {
		if sk.ID == id {
			oldName = sk.Name
			if name != oldName {
				for _, other := range pd.Skills {
					if other.ID != id && other.Name == name {
						return fmt.Errorf("skill %q already exists", name)
					}
				}
			}
			pd.Skills[i].Name = name
			pd.Skills[i].Description = description
			pd.Skills[i].UpdatedAt = time.Now().UnixMilli()
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("skill %s not found", id)
	}
	if err := writePluginsData(pd); err != nil {
		return err
	}
	if oldName != name {
		_ = deleteMdFile("skills", oldName)
	}
	if err := writeMdFile("skills", name, body); err != nil {
		return err
	}
	log.Printf("[plugins] updated skill %s: %s", id, name)
	return nil
}

func (a *App) DeleteSkill(id string) error {
	pluginsMu.Lock()
	defer pluginsMu.Unlock()
	pd, err := readPluginsData()
	if err != nil {
		return err
	}
	filtered := make([]PluginSkill, 0, len(pd.Skills))
	var deletedName string
	for _, sk := range pd.Skills {
		if sk.ID == id {
			deletedName = sk.Name
			continue
		}
		filtered = append(filtered, sk)
	}
	if deletedName == "" {
		return fmt.Errorf("skill %s not found", id)
	}
	pd.Skills = filtered
	if err := writePluginsData(pd); err != nil {
		return err
	}
	_ = deleteMdFile("skills", deletedName)
	log.Printf("[plugins] deleted skill %s (%s)", id, deletedName)
	return nil
}

// ─── Helpers for AI integration ───

func (a *App) GetCommandByName(name string) (string, error) {
	pluginsMu.Lock()
	defer pluginsMu.Unlock()
	pd, err := readPluginsData()
	if err != nil {
		return "", err
	}
	for _, cmd := range pd.Commands {
		if cmd.Name == name {
			body, err := readMdFile("commands", cmd.Name)
			if err != nil {
				return "", err
			}
			return body, nil
		}
	}
	return "", fmt.Errorf("command %q not found", name)
}

func (a *App) GetAllSkillBodies() (string, error) {
	pluginsMu.Lock()
	defer pluginsMu.Unlock()
	pd, err := readPluginsData()
	if err != nil {
		return "", err
	}
	if len(pd.Skills) == 0 {
		return "", nil
	}
	var parts []string
	for _, sk := range pd.Skills {
		body, err := readMdFile("skills", sk.Name)
		if err != nil {
			log.Printf("[plugins] warning: failed to read skill %s: %v", sk.Name, err)
			continue
		}
		if body != "" {
			parts = append(parts, body)
		}
	}
	if len(parts) == 0 {
		return "", nil
	}
	return strings.Join(parts, "\n\n---\n\n"), nil
}

func (a *App) ListCommandNames() ([]string, error) {
	pluginsMu.Lock()
	defer pluginsMu.Unlock()
	pd, err := readPluginsData()
	if err != nil {
		return nil, err
	}
	names := make([]string, len(pd.Commands))
	for i, cmd := range pd.Commands {
		names[i] = cmd.Name
	}
	return names, nil
}

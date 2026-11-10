package core

import (
	"fmt"
	"strings"

	"github.com/rushikesh/devhub/internal/client"
	"github.com/rushikesh/devhub/internal/registry"
)

func (s *Service) AddProject(name, path string) error {
	if s == nil || s.Cfg == nil {
		return fmt.Errorf("service is not configured")
	}
	if err := s.Cfg.AddProject(name, path); err != nil {
		s.Audit(Caller(), "project.add", name, err.Error())
		return err
	}
	s.Audit(Caller(), "project.add", name, "ok")
	return s.syncDaemon()
}

func (s *Service) RemoveProject(name string, killSessions bool) error {
	if s == nil || s.Cfg == nil {
		return fmt.Errorf("service is not configured")
	}
	if killSessions {
		s.stopProjectSessions(name)
	}
	if err := s.Cfg.RemoveProject(name); err != nil {
		s.Audit(Caller(), "project.rm", name, err.Error())
		return err
	}
	s.Audit(Caller(), "project.rm", name, "ok")
	return s.syncDaemon()
}

func (s *Service) RenameProject(old, newName string) error {
	if s == nil || s.Cfg == nil {
		return fmt.Errorf("service is not configured")
	}
	if err := s.Cfg.RenameProject(old, newName); err != nil {
		s.Audit(Caller(), "project.mv", old, err.Error())
		return err
	}
	s.Audit(Caller(), "project.mv", old+"->"+newName, "ok")
	return s.syncDaemon()
}

func (s *Service) ProjectHasSessions(name string) bool {
	if s == nil || s.DB == nil {
		return false
	}
	rows, err := s.DB.List(true)
	if err != nil {
		return false
	}
	for _, r := range rows {
		if r.Project == name && r.Status == registry.StatusRunning {
			return true
		}
	}
	return false
}

func (s *Service) stopProjectSessions(name string) {
	if s.DB == nil {
		return
	}
	rows, err := s.DB.List(true)
	if err != nil {
		return
	}
	for _, r := range rows {
		if r.Project != name {
			continue
		}
		_ = s.Stop(fmt.Sprintf("%s/%s", r.Project, r.Name))
	}
}

func (s *Service) syncDaemon() error {
	if s.Cfg == nil {
		return nil
	}
	c := s.Remote
	if c == nil {
		c = client.Unix(s.Cfg, Caller())
	}
	if c.Healthy() {
		if err := c.Reload(); err != nil {
			return err
		}
		return nil
	}
	if s.Tmux != nil {
		_ = s.ApplyKeybindings()
	}
	return nil
}

func UniqueAddName(cfgProjects map[string]string, base string, reserved map[string]string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "project"
	}
	taken := func(n string) bool {
		if cfgProjects != nil {
			if _, ok := cfgProjects[n]; ok {
				return true
			}
		}
		if reserved != nil {
			if _, ok := reserved[n]; ok {
				return true
			}
		}
		return false
	}
	if !taken(base) {
		return base
	}
	for i := 2; ; i++ {
		cand := fmt.Sprintf("%s-%d", base, i)
		if !taken(cand) {
			return cand
		}
	}
}

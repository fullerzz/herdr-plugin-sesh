package sources

import (
	"context"
	"regexp"
	"slices"
	"sync"

	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
)

func Merge(ctx context.Context, srcs []Source, order []string, blacklist []string, onlyBlacklisted, dedupe bool) (model.Sessions, error) {
	by := map[string]model.Sessions{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	var first error
	for _, src := range srcs {
		wg.Add(1)
		go func(s Source) {
			defer wg.Done()
			got, err := s.List(ctx)
			mu.Lock()
			defer mu.Unlock()
			if err != nil && first == nil {
				first = err
			}
			by[s.Name()] = got
		}(src)
	}
	wg.Wait()
	if first != nil {
		return model.Sessions{}, first
	}
	ordered := append([]string{}, order...)
	seenOrder := map[string]bool{}
	for _, o := range ordered {
		seenOrder[o] = true
	}
	for _, s := range srcs {
		if !seenOrder[s.Name()] {
			ordered = append(ordered, s.Name())
		}
	}
	out := model.NewSessions()
	seenName := map[[2]string]string{}
	bl := compile(blacklist)
	for _, name := range ordered {
		ss := by[name]
		for _, sess := range ss.Ordered() {
			isbl := isBlacklisted(bl, sess.Name)
			if len(bl) > 0 || onlyBlacklisted {
				if isbl != onlyBlacklisted {
					continue
				}
			}
			if dedupe {
				key := sess.Name
				if key == "" {
					key = sess.Path
				}
				identity := [2]string{sess.Machine, key}
				if kept, ok := seenName[identity]; ok {
					// The open remote space stands in for its configured entry,
					// whichever source comes first.
					if sess.Machine != "" && sess.Source == "config" {
						s := out.Directory[kept]
						s.Configured = true
						out.Directory[kept] = s
					} else if sess.Machine != "" && sess.WorkspaceID != "" && out.Directory[kept].Source == "config" {
						sess.Configured = true
						seenName[identity] = replace(&out, kept, sess)
					}
					continue
				}
				seenName[identity] = out.Add(sess)
				continue
			}
			out.Add(sess)
		}
	}
	return out, nil
}

// replace puts s in old's place in the display order.
func replace(ss *model.Sessions, old string, s model.Session) string {
	key := model.Key(s)
	delete(ss.Directory, old)
	ss.Directory[key] = s
	ss.OrderedIndex[slices.Index(ss.OrderedIndex, old)] = key
	return key
}
func compile(items []string) []*regexp.Regexp {
	out := []*regexp.Regexp{}
	for _, it := range items {
		if re, err := regexp.Compile(it); err == nil {
			out = append(out, re)
		}
	}
	return out
}
func isBlacklisted(res []*regexp.Regexp, name string) bool {
	for _, re := range res {
		if re.MatchString(name) {
			return true
		}
	}
	return false
}

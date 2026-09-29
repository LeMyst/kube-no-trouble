package judge

import (
	"context"
	"errors"
	"fmt"

	"github.com/LeMyst/kube-no-trouble/pkg/rules"
	"github.com/open-policy-agent/opa/rego"
	"github.com/rs/zerolog/log"
)

type RegoJudge struct {
	preparedQuery rego.PreparedEvalQuery
}

type RegoOpts struct {
}

func NewRegoJudge(opts *RegoOpts, rulesList []rules.Rule) (*RegoJudge, error) {
	ctx := context.Background()

	regoOpts := []rego.Option{
		rego.Query("data[_].main"),
	}

	for _, info := range rulesList {
		regoOpts = append(regoOpts, rego.Module(info.Name, info.Rule))
		log.Info().Str("name", info.Name).Msg("Loaded ruleset")
	}

	r := rego.New(regoOpts...)
	pq, err := r.PrepareForEval(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare rego bundle: %w", err)
	}

	judge := &RegoJudge{preparedQuery: pq}
	return judge, nil
}

func (j *RegoJudge) Eval(input []map[string]interface{}) ([]Result, error) {
	if j == nil || j.preparedQuery == nil {
		return nil, errors.New("rego judge is not initialized")
	}

	ctx := context.Background()

	log.Trace().Msgf("evaluating +%v", input)
	rs, err := j.preparedQuery.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return nil, fmt.Errorf("rego eval failed: %w", err)
	}

	results := []Result{}
	for _, r := range rs {
		for _, e := range r.Expressions {
			list, ok := e.Value.([]interface{})
			if !ok {
				log.Debug().Msgf("unexpected expression value type: %T", e.Value)
				continue
			}

			for _, item := range list {
				m, ok := item.(map[string]interface{})
				if !ok {
					log.Debug().Msgf("unexpected item type in eval result: %T", item)
					continue
				}

				log.Trace().Msgf("parsing +%v", m)

				sinceStr, _ := m["Since"].(string)
				since, err := NewVersion(sinceStr)
				if err != nil && sinceStr != "" {
					log.Debug().Msgf("Failed to parse version: %s", err)
				}

				name, _ := m["Name"].(string)
				namespace, _ := m["Namespace"].(string)
				kind, _ := m["Kind"].(string)
				apiVersion, _ := m["ApiVersion"].(string)
				replaceWith, _ := m["ReplaceWith"].(string)
				ruleSet, _ := m["RuleSet"].(string)

				if namespace == "" {
					log.Warn().Msgf("Object has invalid namespace: %s/%s %s", apiVersion, kind, name)
					namespace = "<undefined>"
				}

				var labels map[string]interface{}
				if v, ok := m["Labels"].(map[string]interface{}); ok {
					labels = v
				} else {
					labels = make(map[string]interface{})
				}

				results = append(results, Result{
					Name:        name,
					Namespace:   namespace,
					Kind:        kind,
					ApiVersion:  apiVersion,
					ReplaceWith: replaceWith,
					RuleSet:     ruleSet,
					Since:       since,
					Labels:      labels,
				})
			}
		}
	}

	return results, nil
}

package bench

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

type sessionDemand struct {
	nodes []*graphNode
	count int
}

func graphSessionResources(g *Graph, selected map[*graphNode]bool) map[string][]sessionDemand {
	resources := make(map[string][]sessionDemand)
	owners := make(map[*graphSession]int)

	for _, node := range g.nodes {
		options, _ := graphStepOptions(g.execution, node)
		if !selected[node] || !g.execution.Enabled(node.name) || !options.retained {
			continue
		}

		database := options.driver.name
		if database == "default" {
			database = ""
		}

		if node.session == nil {
			resources[database] = append(resources[database], sessionDemand{
				nodes: []*graphNode{node}, count: options.policy.workers,
			})

			continue
		}

		index, exists := owners[node.session]
		if !exists {
			index = len(resources[database])
			owners[node.session] = index

			resources[database] = append(resources[database], sessionDemand{count: 1})
		}

		resources[database][index].nodes = append(resources[database][index].nodes, node)
	}

	return resources
}

func (g *Graph) hasConcurrentNodes(selected map[*graphNode]bool) bool {
	for index, node := range g.nodes {
		if !selected[node] || !g.execution.Enabled(node.name) || node.join {
			continue
		}

		for _, other := range g.nodes[:index] {
			if selected[other] && g.execution.Enabled(other.name) && !other.join &&
				!graphCompletionPath(node, other, make(map[*graphNode]bool)) &&
				!graphCompletionPath(other, node, make(map[*graphNode]bool)) {
				return true
			}
		}
	}

	return false
}

func validateGraphCapacity(g *Graph, selected map[*graphNode]bool) error {
	for database, demands := range graphSessionResources(g, selected) {
		configuration := g.execution.configuration(database)

		limit := configuration.sessionLimit()
		if limit > 0 && maximumSessionDemand(demands) > limit {
			return fmt.Errorf("%w: graph database %q needs more than %d concurrent retained sessions",
				ErrSessionCapacity, database, limit)
		}
	}

	return nil
}

func (g *Graph) preflightSessions(selected map[*graphNode]bool) error {
	for database, demands := range graphSessionResources(g, selected) {
		backend, _, err := g.execution.database(g.execution.ctx, database)
		if err != nil {
			return err
		}

		provider, ok := backend.(driver.SessionProvider)
		if !ok {
			return ErrSessionUnsupported
		}

		limit, demand := provider.SessionCapacity(), maximumSessionDemand(demands)
		if limit > 0 && demand > limit {
			return fmt.Errorf("%w: graph database %q needs %d concurrent retained sessions, pool has %d",
				ErrSessionCapacity, database, demand, limit)
		}
	}

	return nil
}

func sessionDemandBefore(first, second sessionDemand) bool {
	for _, after := range second.nodes {
		for _, before := range first.nodes {
			if after == before || !graphCompletionPath(after, before, make(map[*graphNode]bool)) {
				return false
			}
		}
	}

	return true
}

// Weighted Dilworth computes the largest concurrent session demand. Summing all
// nodes incomparable with one node would also add mutually sequential work.
func maximumSessionDemand(demands []sessionDemand) int {
	n := len(demands)
	source := n + n
	sink := source + 1

	capacity := make([][]int, sink+1)
	for index := range capacity {
		capacity[index] = make([]int, sink+1)
	}

	total := 0
	for _, demand := range demands {
		if demand.count > int(^uint(0)>>1)-total {
			return int(^uint(0) >> 1)
		}

		total += demand.count
	}

	for first, demand := range demands {
		capacity[source][first] = demand.count

		capacity[n+first][sink] = demand.count
		for second := range demands {
			if first != second && sessionDemandBefore(demand, demands[second]) {
				capacity[first][n+second] = total
			}
		}
	}

	return total - sessionMatchingFlow(capacity, source, sink)
}

func sessionMatchingFlow(capacity [][]int, source, sink int) int {
	flow := 0

	for {
		previous := make([]int, len(capacity))
		for index := range previous {
			previous[index] = -1
		}

		previous[source] = source

		queue := []int{source}
		for len(queue) > 0 && previous[sink] < 0 {
			current := queue[0]
			queue = queue[1:]

			for next, remaining := range capacity[current] {
				if remaining > 0 && previous[next] < 0 {
					previous[next] = current
					queue = append(queue, next)
				}
			}
		}

		if previous[sink] < 0 {
			return flow
		}

		amount := capacity[previous[sink]][sink]
		for next := sink; next != source; next = previous[next] {
			amount = min(amount, capacity[previous[next]][next])
		}

		for next := sink; next != source; next = previous[next] {
			capacity[previous[next]][next] -= amount
			capacity[next][previous[next]] += amount
		}

		flow += amount
	}
}

func (c *DriverConfig) sessionLimit() int {
	switch c.Kind {
	case DriverPostgres, DriverCRDB, DriverYugabyte:
		if c.Postgres != nil && c.Postgres.MaxConns != nil {
			return int(*c.Postgres.MaxConns)
		}

		if parsed, err := url.Parse(c.URL); err == nil {
			limit, _ := strconv.Atoi(parsed.Query().Get("pool_max_conns"))

			return limit
		}
	case DriverMySQL, DriverTiDB:
		if c.SQL != nil && c.SQL.MaxOpenConns != nil {
			return int(*c.SQL.MaxOpenConns)
		}
	case DriverPicodata, DriverYDB, DriverNoop, DriverCSV, DriverRecording:
		return 0
	}

	return 0
}

package bench

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"
)

// Graph declares work without changing the immediate semantics of Execution.Step.
// Run freezes and validates the selected plan before invoking any action.
type Graph struct {
	execution    *Execution
	name         string
	nodes        []*graphNode
	modes        []graphMode
	prefix       string
	owner        *Graph
	frozen       bool
	measurements []graphMeasurement
	sessions     []*graphSession
}

type graphNode struct {
	name            string
	action          Action
	options         []StepOption
	dependencies    []graphDependency
	cleanup         time.Duration
	auxiliary       *graphNode
	ready           bool
	optionalCleanup bool
	before          func()
	after           func(Result)
	session         *graphSession
	join            bool
}

type graphDependency struct {
	node         *graphNode
	event        string
	beforeAction bool
}

type graphMeasurement struct {
	name  string
	nodes []*graphNode
}

// Measure records an enclosing action window for explicitly named participants;
// child counters remain child counters and are never added a second time.
func (g *Graph) Measure(name string, nodes ...NodeRef) {
	root := g.root()
	if root.frozen || name == "" || len(nodes) == 0 {
		invalid("graph measurement", inputError("name and participants required"))
	}

	window := graphMeasurement{name: g.prefix + name}

	for _, node := range nodes {
		if node.graph != root || node.node == nil {
			invalid("graph measurement", inputError("foreign participant"))
		}

		window.nodes = append(window.nodes, node.node)
	}

	root.measurements = append(root.measurements, window)
}

type graphMode struct {
	name, description string
	members           []*graphNode
	err               error
}

// ModeRef attaches input-resolved eligibility without executing preflight work.
type ModeRef struct {
	graph *Graph
	index int
}

func (m ModeRef) Require(err error) ModeRef {
	if m.graph == nil || m.graph.frozen {
		invalid("graph mode", inputError("invalid or frozen mode"))
	}

	m.graph.modes[m.index].err = errors.Join(m.graph.modes[m.index].err, err)

	return m
}

// NodeRef belongs to one graph declaration; it cannot be used across definitions.
type NodeRef struct {
	graph *Graph
	node  *graphNode
}

// SessionRef preserves one connection across sequential graph nodes. Sharing a
// session between concurrent nodes is rejected before execution.
type SessionRef struct {
	graph   *Graph
	session *graphSession
}

func (g *Graph) Session(name string, driver DriverRef) SessionRef {
	root := g.root()
	if root.frozen || name == "" || (driver.owner != nil && driver.owner != root.execution.def) {
		invalid("graph session", inputError("name and graph-owned driver are required"))
	}

	session := &graphSession{name: g.prefix + name, driver: driver}
	root.sessions = append(root.sessions, session)

	return SessionRef{root, session}
}

func (n NodeRef) UseSession(session SessionRef) NodeRef {
	if n.graph == nil || n.graph.frozen || session.graph != n.graph || session.session == nil {
		invalid("graph session", inputError("session must belong to the same unfrozen graph"))
	}

	n.node.session = session.session

	return n
}

// GraphDescription is the resolved topology, independent of runtime outcomes.
type GraphDescription struct {
	Name         string                   `json:"name"`
	Mode         string                   `json:"mode"`
	Modes        []ModeDescription        `json:"modes"`
	Nodes        []NodeDescription        `json:"nodes"`
	Measurements []MeasurementDescription `json:"measurements,omitempty"`
}

type MeasurementDescription struct {
	Name  string   `json:"name"`
	Nodes []string `json:"nodes"`
}

type ModeDescription struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Available   bool     `json:"available"`
	Reason      string   `json:"reason,omitempty"`
	Nodes       []string `json:"nodes"`
}

type NodeDescription struct {
	Name             string                  `json:"name"`
	Selected         bool                    `json:"selected"`
	Dependencies     []DependencyDescription `json:"dependencies,omitempty"`
	Cleanup          bool                    `json:"cleanup,omitempty"`
	AuxiliaryFor     string                  `json:"auxiliary_for,omitempty"`
	Join             bool                    `json:"join,omitempty"`
	RetainedSessions bool                    `json:"retained_sessions,omitempty"`
	Session          string                  `json:"session,omitempty"`
	Driver           string                  `json:"driver,omitempty"`
	SignalsReady     bool                    `json:"signals_ready,omitempty"`
	CleanupTimeout   time.Duration           `json:"cleanup_timeout,omitempty"`
	OptionalCleanup  bool                    `json:"optional_cleanup,omitempty"`
	Iterations       int64                   `json:"iterations,omitempty"`
	Duration         time.Duration           `json:"duration,omitempty"`
	DrainTimeout     time.Duration           `json:"drain_timeout,omitempty"`
	DrainUnlimited   bool                    `json:"drain_unlimited,omitempty"`
	Policy           StepDescription         `json:"policy"`
}

type DependencyDescription struct {
	Node         string `json:"node"`
	Event        string `json:"event"`
	BeforeAction bool   `json:"before_action,omitempty"`
}

func (e *Execution) Graph(name string) *Graph {
	if name == "" {
		invalid("graph", inputError("name is required"))
	}

	return &Graph{execution: e, name: name}
}

func (g *Graph) root() *Graph {
	if g.owner != nil {
		return g.owner
	}

	return g
}

func (g *Graph) Node(name string, action Action, options ...StepOption) NodeRef {
	root := g.root()
	if root.frozen || name == "" || action == nil || strings.Contains(name, "/") {
		invalid("graph node", inputError("unfrozen graph, local name and action are required"))
	}

	node := &graphNode{name: g.prefix + name, action: action, options: slices.Clone(options)}
	root.nodes = append(root.nodes, node)

	return NodeRef{graph: root, node: node}
}

// Join is a success barrier without a worker, database action, or progress weight.
func (g *Graph) Join(name string, nodes ...NodeRef) NodeRef {
	if len(nodes) == 0 {
		invalid("graph join", inputError("participants are required"))
	}

	node := g.Node(name, func(context.Context, *Bench) error { return nil }).AfterSuccess(nodes...)
	node.node.join = true

	return node
}

func (g *Graph) WorkerNode(name string, initialize WorkerInit, options ...StepOption) NodeRef {
	if initialize == nil {
		invalid("graph worker", inputError("initializer is required"))
	}

	options = append(slices.Clone(options), stepOption(func(o *stepOptions) { o.initialize = initialize }))

	return g.Node(name, func(context.Context, *Bench) error { return nil }, options...)
}

// Group namespaces an ordinary declaration helper; it never creates a worker or
// adds synthetic work to progress. Returned leaves can be composed into modes.
func (g *Graph) Group(name string, declare func(*Graph)) []NodeRef {
	if name == "" || strings.Contains(name, "/") || declare == nil {
		invalid("graph group", inputError("local name and declaration are required"))
	}

	root := g.root()
	start := len(root.nodes)
	declare(&Graph{execution: g.execution, name: g.name, owner: root, prefix: g.prefix + name + "/"})

	refs := make([]NodeRef, 0, len(root.nodes)-start)
	for _, node := range root.nodes[start:] {
		refs = append(refs, NodeRef{root, node})
	}

	return refs
}

func (n NodeRef) after(event string, beforeAction bool, predecessors []NodeRef) NodeRef {
	if n.graph == nil || n.node == nil || n.graph.frozen {
		invalid("graph dependency", inputError("invalid or frozen node"))
	}

	for _, predecessor := range predecessors {
		if predecessor.graph != n.graph || predecessor.node == nil {
			invalid("graph dependency", inputError("nodes belong to different graphs"))
		}

		n.node.dependencies = append(n.node.dependencies, graphDependency{predecessor.node, event, beforeAction})
	}

	return n
}

// After orders selected nodes without requiring an excluded prerequisite to run.
// Use AfterSuccess when a missing or failed prerequisite makes execution invalid.
func (n NodeRef) After(nodes ...NodeRef) NodeRef { return n.after("ordered", false, nodes) }

func (n NodeRef) AfterSuccess(nodes ...NodeRef) NodeRef { return n.after("succeeded", false, nodes) }

func (n NodeRef) AfterCompletion(nodes ...NodeRef) NodeRef { return n.after("completed", false, nodes) }

func (n NodeRef) AfterStarted(nodes ...NodeRef) NodeRef { return n.after("started", false, nodes) }
func (n NodeRef) AfterReady(nodes ...NodeRef) NodeRef   { return n.after("ready", false, nodes) }

// RunAfterReady and RunAfterSuccess allow initialization before the dependency,
// but hold the action window until it is satisfied.
func (n NodeRef) RunAfterReady(nodes ...NodeRef) NodeRef { return n.after("ready", true, nodes) }

func (n NodeRef) RunAfterSuccess(nodes ...NodeRef) NodeRef { return n.after("succeeded", true, nodes) }

// SignalsReady requires the action to call Bench.Ready. Worker initialization
// alone does not imply application readiness for this node.
func (n NodeRef) SignalsReady() NodeRef {
	if n.graph == nil || n.graph.frozen {
		invalid("graph readiness", inputError("invalid or frozen node"))
	}

	n.node.ready = true

	return n
}

// Finally runs once after its scope's work has joined, even after cancellation.
func (n NodeRef) Finally(timeout time.Duration, owners ...NodeRef) NodeRef {
	if timeout <= 0 || len(owners) == 0 {
		invalid("graph cleanup", inputError("positive timeout and owners are required"))
	}

	n.node.cleanup = timeout

	return n.AfterCompletion(owners...)
}

// While makes a long-lived auxiliary node owned by foreground work. The graph
// cancels it when its owner finishes, then joins it before dependent cleanup.
func (n NodeRef) While(owner NodeRef) NodeRef {
	if n.graph == nil || n.graph.frozen || owner.graph != n.graph || owner.node == nil {
		invalid("graph auxiliary", inputError("owner must belong to the same unfrozen graph"))
	}

	n.node.auxiliary = owner.node

	return n
}

// Before runs once after worker initialization, immediately before the action
// window. It is not called during discovery or for skipped/blocked nodes.
func (n NodeRef) Before(action func()) NodeRef {
	if n.graph == nil || n.graph.frozen || action == nil {
		invalid("graph boundary", inputError("invalid before action"))
	}

	n.node.before = action

	return n
}

// AfterResult observes the final node outcome after workers and cleanup join.
func (n NodeRef) AfterResult(action func(Result)) NodeRef {
	if n.graph == nil || n.graph.frozen || action == nil {
		invalid("graph boundary", inputError("invalid result action"))
	}

	n.node.after = action

	return n
}

func (g *Graph) Sequence(nodes ...NodeRef) {
	root := g.root()
	for _, node := range nodes {
		if node.graph != root || node.node == nil {
			invalid("graph sequence", inputError("foreign node"))
		}
	}

	for index := 1; index < len(nodes); index++ {
		nodes[index].AfterSuccess(nodes[index-1])
	}
}

// Mode selects reusable nodes. Dependencies are included before operator filters
// are applied; filters never silently restore excluded prerequisite work.
func (g *Graph) Mode(name, description string, nodes ...NodeRef) ModeRef {
	root := g.root()
	if root.frozen || name == "" || description == "" {
		invalid("graph mode", inputError("name and description are required"))
	}

	mode := graphMode{name: name, description: description}

	for _, node := range nodes {
		if node.graph != root || node.node == nil {
			invalid("graph mode", inputError("node belongs to another graph"))
		}

		mode.members = append(mode.members, node.node)
	}

	root.modes = append(root.modes, mode)

	return ModeRef{root, len(root.modes) - 1}
}

func graphStepOptions(e *Execution, node *graphNode) (options stepOptions, err error) {
	defer recoverValidation(&err)

	for _, option := range node.options {
		if option == nil {
			return options, inputError("nil graph step option")
		}

		option.applyStep(&options)
	}

	if options.policy.mode == "" {
		options.policy = Policy{mode: "once", workers: 1}
	}

	if options.policy.workers < 1 || (options.driver.owner != nil && options.driver.owner != e.def) {
		return options, inputError("invalid graph policy or driver")
	}

	if node.session != nil {
		if options.driver.owner != nil && options.driver.name != node.session.driver.name {
			return options, inputError("graph session driver conflicts with node driver")
		}

		options.driver = node.session.driver
		options.retained = true
	}

	if options.always {
		if node.cleanup == 0 {
			node.cleanup = options.cleanup
			node.optionalCleanup = true
		}
	}

	if node.cleanup > 0 && options.policy.mode != "once" {
		return options, inputError("graph cleanup must run once")
	}

	return options, validateGraphJoin(node)
}

func validateGraphJoin(node *graphNode) error {
	if !node.join {
		return nil
	}

	for _, dependency := range node.dependencies {
		if dependency.beforeAction {
			return inputError("graph join cannot have action-only dependencies; use AfterReady or AfterSuccess")
		}
	}

	if node.session != nil || node.ready || node.cleanup > 0 || node.auxiliary != nil ||
		node.before != nil || node.after != nil {
		return inputError("graph join cannot own worker lifecycle or resources")
	}

	return nil
}

// DescribeGraphs observes graph declarations using the same input resolution as
// RunTest. Existing Description remains source-compatible for external literals.
//
//nolint:gocritic // public run inputs are copied values, matching ResolveRun and RunTest.
func DescribeGraphs(test Test, options RunOptions) ([]GraphDescription, error) {
	d, err := observeSelected(test, options.Params, copyDriverConfigs(options.Drivers),
		false, options.Steps, options.NoSteps)
	if err != nil {
		return nil, err
	}

	return slices.Clone(d.Execution.graphs), nil
}

func (g *Graph) Run(mode string) error {
	g = g.root()
	if g.frozen {
		return inputError("graph %q has already run", g.name)
	}

	description, selected, err := g.resolve(mode)
	if err != nil {
		return fmt.Errorf("graph %q: %w", g.name, err)
	}

	if g.execution.root != nil {
		index := len(g.execution.graphs)
		if index >= len(g.execution.expectedGraphs) || !reflect.DeepEqual(description, g.execution.expectedGraphs[index]) {
			return inputError("graph %q changed between observation and execution", g.name)
		}
	}

	g.execution.graphs = append(g.execution.graphs, description)
	for index := range description.Nodes {
		node := &description.Nodes[index]
		if node.Selected && !node.Join {
			g.execution.observed = append(g.execution.observed, node.Policy)
		}
	}

	if g.execution.root == nil {
		return nil
	}

	if err := g.preflightSessions(selected); err != nil {
		g.execution.err = errors.Join(g.execution.err, err)

		return err
	}

	return g.run(selected)
}

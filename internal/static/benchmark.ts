// This file is used for running in stroppy K6 executor
// Feel free to modify it to your needs of benchmarks
// For more info please refer to https://github.com/stroppy-io/stroppy

import {Counter, Trend} from "k6/metrics";
import {Options, Scenario} from "k6/options";
import stroppy from "k6/x/stroppy";
import {Duration, K6Scenario, RampingArrivalRate_RateStage, RampingVUs_VUStage, StepContext} from "./stroppy.pb.js";

export type ProtoSerialized<T extends any> = string;

export interface StroppyXk6Instance {
    setup(config: ProtoSerialized<StepContext>): Error | undefined;

    runTransaction(): Error | undefined;

    teardown(): Error | undefined;
}

function secondsStringDuration(seconds: number): string {
    return seconds + "s";
}

function numberOrDefault(value: any, defaultValue: number): number {
    if (value === undefined) {
        return defaultValue;
    }
    return isNaN(Number(value)) === true ? defaultValue : Number(value);
}

function durationOrDefault({value, defaultValue}: { value?: Duration, defaultValue?: number }): string {
    if (value === null || value === undefined) {
        return secondsStringDuration(defaultValue ? defaultValue : 1);
    }
    return secondsStringDuration(Number(value.seconds));
}

export const INSTANCE: StroppyXk6Instance = stroppy.new();

// passed from Golang execution
export const STROPPY_CONTEXT: StepContext = StepContext.fromJsonString(
    __ENV.context,
);
if (!STROPPY_CONTEXT) {
    throw new Error("Please define step run config (-econtext={...})");
}

export const METER_SETUP_TIME_COUNTER = new Counter("setup_time");
export const METER_REQUESTS_COUNTER = new Counter("total_requests");
export const METER_REQUEST_ERROR_COUNTER = new Counter("total_errors");
export const METER_RESPONSES_TIME_TREND = new Trend("response_time");


export const options: Options = {
    setupTimeout: durationOrDefault({value: STROPPY_CONTEXT.executor.k6.setupTimeout, defaultValue: 60}),
    tags: {
        runId: STROPPY_CONTEXT.config.runId,
        // TODO: add benchmark name in config
        // benchmark: STROPPY_CONTEXT.benchmark.name,
        step: STROPPY_CONTEXT.workload.name,
        // ...STROPPY_CONTEXT.config.metadata uncomment if needed pass metadata in metrics labels
    },
    scenarios: {
        scenario: protoScenarioToK6(STROPPY_CONTEXT.executor.k6.scenario)
    },
    // TODO: think about thresholds, now to supported cause hasn't duration in config
    // thresholds: {
    //     total_errors: [
    //         {
    //             threshold: `count < ${K6_DEFAULT_ERROR_THRESHOLD}`,
    //             abortOnFail: true,
    //         },
    //     ],
    // },
};

// This object will be created in setup function
// and passed to "default" function as argument by k6
class Context {
}

// @ts-ignore
export const setup = (): Context => {
    // this metric must be initialized before benchmark execution
    METER_REQUEST_ERROR_COUNTER.add(0);

    const startTime = Date.now();
    console.log(STROPPY_CONTEXT);

    const err = INSTANCE.setup(StepContext.toJsonString(STROPPY_CONTEXT));
    if (err !== undefined) {
        throw err;
    }

    METER_SETUP_TIME_COUNTER.add(Date.now() - startTime);

    return <Context>{};
};

export default (ctx: Context) => {
    const metricsTags = {
        // "tx_name": transaction.name // TODO: add name field to transaction in proto
    };
    // TODO: add driver metrics
    const startTime = Date.now();
    const err = INSTANCE.runTransaction();
    if (err) {
        METER_REQUEST_ERROR_COUNTER.add(1, metricsTags);
    }
    METER_REQUESTS_COUNTER.add(1, metricsTags);
    METER_RESPONSES_TIME_TREND.add(Date.now() - startTime, metricsTags);
};

export const teardown = () => {
    const err = INSTANCE.teardown();
    if (err !== undefined) {
        throw err;
    }
};

// Summary function, that will create summary file with metrics.
export function handleSummary(runResult: RunResult<Context>) {
    return {
        stdout: resultToJsonString<Context>(runResult, {some: "baggage"}),
    };
}


interface CounterMeter {
    values: { count: number; rate: number };
}

interface TrendMeter {
    values: {
        avg: number;
        min: number;
        med: number;
        max: number;
        p90: number;
        p95: number;
    };
}

class RunResult<T extends any> {
    setup_data: T;
    metrics: {
        data_received: CounterMeter;
        iteration_duration: TrendMeter;
        dropped_iterations?: CounterMeter;
        iterations: CounterMeter;
        data_sent: CounterMeter;

        // Custom metrics
        setup_time: CounterMeter;
        total_requests: CounterMeter;
        total_errors: CounterMeter;
        response_time: TrendMeter;
    };
    state: {
        isStdOutTTY: boolean;
        isStdErrTTY: boolean;
        testRunDurationMs: number;
    };
}

function resultToJsonString<T extends any>(
    result: RunResult<T>,
    baggage?: { [name: string]: any },
) {
    const testDuration =
        (result.state.testRunDurationMs -
            result.metrics.setup_time.values.count) /
        1000;
    const output = {
        runId: STROPPY_CONTEXT.config.runId,
        // TODO: add benchmark name in config
        // benchmark: STROPPY_CONTEXT.globalConfig.benchmark.name,
        step: STROPPY_CONTEXT.workload.name,
        seed: STROPPY_CONTEXT.config.seed,
        date: new Date().toLocaleString(),
        ...baggage,
        setupData: result.setup_data,
        metadata: {...STROPPY_CONTEXT.config.metadata},
        scenario: {...STROPPY_CONTEXT.executor.k6.scenario},
        k6_options: {...options},
        durationAllStagesSec: Number(
            (result.state.testRunDurationMs / 1000).toFixed(5),
        ),
        durationTestSec: testDuration,
        requestsProcessed: result.metrics.total_requests.values.count,
        totalErrors: result.metrics.total_errors.values.count,
        droppedIterations:
            "dropped_iterations" in result.metrics
                ? {
                    count: result.metrics.dropped_iterations.values.count,
                    rate: result.metrics.dropped_iterations.values.rate,
                }
                : {
                    count: 0,
                    rate: 0,
                },
        rps: {
            actual: Number(
                (
                    result.metrics.total_requests.values.count / testDuration
                ).toFixed(5),
            ),
            actual_success: Number(
                (
                    (result.metrics.total_requests.values.count -
                        result.metrics.total_errors.values.count) /
                    testDuration
                ).toFixed(3),
            ),
            target: -1,
        },
        responseTime:
            "response_time" in result.metrics
                ? {
                    minSec: result.metrics.response_time.values.min / 1000,
                    maxSec: result.metrics.response_time.values.max / 1000,
                    avgSec: Number(
                        (
                            result.metrics.response_time.values.avg / 1000
                        ).toFixed(5),
                    ),
                }
                : {
                    minSec: -1,
                    maxSec: -1,
                    avgSec: -1,
                },
    };
    return JSON.stringify(output, null, 2)
        .replace(/"/g, "")
        .replace(/(\n\s*\n)+/g, "\n");
}

// @ts-ignore
function protoScenarioToK6(scenario: K6Scenario): Scenario {
    switch (scenario.executor.oneofKind) {
        case "sharedIterations":
            return {
                executor: "shared-iterations",
                vus: scenario.executor.sharedIterations.vus,
                iterations: numberOrDefault(scenario.executor.sharedIterations.iterations, 1)
            }
        case "perVuIterations":
            return {
                executor: "shared-iterations",
                vus: scenario.executor.perVuIterations.vus,
                iterations: numberOrDefault(scenario.executor.perVuIterations.iterations, 1)
            }
        case "constantVus":
            return {
                executor: "constant-vus",
                vus: scenario.executor.constantVus.vus,
                duration: durationOrDefault({value: scenario.executor.constantVus.duration, defaultValue: 60})
            }
        case "rampingVus":
            return {
                executor: "ramping-vus",
                stages: scenario.executor.rampingVus.stages.map((stage: RampingVUs_VUStage) => ({
                    duration: durationOrDefault({value: stage.duration}),
                    target: stage.target
                })),
                startVUs: numberOrDefault(scenario.executor.rampingVus.startVus, 1),
            }
        case "constantArrivalRate":
            return {
                executor: "constant-arrival-rate",
                duration: durationOrDefault({value: scenario.executor.constantArrivalRate.duration}),
                rate: scenario.executor.constantArrivalRate.rate,
                timeUnit: durationOrDefault({value: scenario.executor.constantArrivalRate.timeUnit}),
                preAllocatedVUs: numberOrDefault(scenario.executor.constantArrivalRate.preAllocatedVus, 1),
                maxVUs: numberOrDefault(scenario.executor.constantArrivalRate.maxVus, 1)
            }
        case "rampingArrivalRate":
            return {
                executor: "ramping-arrival-rate",
                maxVUs: numberOrDefault(scenario.executor.rampingArrivalRate.maxVus, 1),
                stages: scenario.executor.rampingArrivalRate.stages.map((stage: RampingArrivalRate_RateStage) => ({
                    duration: durationOrDefault({value: stage.duration}),
                    target: stage.target
                })),
                startRate: numberOrDefault(scenario.executor.rampingArrivalRate.startRate, 1),
                timeUnit: durationOrDefault({value: scenario.executor.rampingArrivalRate.timeUnit}),
                preAllocatedVUs: numberOrDefault(scenario.executor.rampingArrivalRate.preAllocatedVus, 1),
            }
    }
}

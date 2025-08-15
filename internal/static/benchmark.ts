// This file is used for running in stroppy K6 executor
// Feel free to modify it to your needs of benchmarks
// For more info please refer to https://github.com/stroppy-io/stroppy

import {Options} from 'k6/options';
import {
    DriverQueriesList,
    DriverQuery,
    INSTANCE,
    K6_DEFAULT_ERROR_THRESHOLD,
    K6_DEFAULT_TIME_UNITS,
    K6_SETUP_TIMEOUT,
    K6_STEP_DURATION,
    K6_STEP_MAX_VUS,
    K6_STEP_PRE_ALLOCATED_VUS,
    K6_STEP_RATE,
    METER_REQUEST_ERROR_COUNTER,
    METER_REQUESTS_COUNTER,
    METER_RESPONSES_TIME_TREND,
    METER_SETUP_TIME_COUNTER,
    ProtoSerialized,
    RunResult,
    StepContext,
    STROPPY_CONTEXT,
} from './stroppy.pb.js';

export const options: Options = {
    setupTimeout: K6_SETUP_TIMEOUT,
    tags: {
        runId: STROPPY_CONTEXT.config.runId,
        benchmark: STROPPY_CONTEXT.benchmark.name,
        step: STROPPY_CONTEXT.step.name,
        // ...STROPPY_CONTEXT.config.metadata uncomment if needed pass metadata in metrics labels
    },
    scenarios: {
        const_rps: {
            // k6 executor used in this mode
            executor: "constant-arrival-rate",
            // How long the test lasts
            duration: K6_STEP_DURATION,
            // How many iterations per timeUnit
            rate: K6_STEP_RATE,
            // Start `rate` iterations per second
            timeUnit: K6_DEFAULT_TIME_UNITS,
            // Pre-allocate VUs
            preAllocatedVUs: K6_STEP_PRE_ALLOCATED_VUS,
            // Max number of VUs can be dynamicly usedisNaN(Number(STEP_RUN_CONTEXT.config.k6Executor)) === true ? -1 : Number(__ENV.vu)
            maxVUs: K6_STEP_MAX_VUS
        }
    },
    thresholds: {
        total_errors: [
            {
                threshold: `count < ${K6_DEFAULT_ERROR_THRESHOLD}`,
                abortOnFail: true
            }
        ]
    }
};

// This object will be created in setup function
// and passed to "default" function as argument by k6
class Context {
    queries: ProtoSerialized<DriverQueriesList>
}

// @ts-ignore
export const setup = (): Context => {
    // this metric must be initialized before benchmark execution
    METER_REQUEST_ERROR_COUNTER.add(0);

    const startTime = Date.now();

    const err = INSTANCE.setup(StepContext.toJsonString(STROPPY_CONTEXT))
    if (err !== undefined) {
        throw err
    }
    const queries = INSTANCE.generateQueue()

    METER_SETUP_TIME_COUNTER.add(Date.now() - startTime)

    return <Context>{
        queries: queries
    }
};

export default (ctx: Context) => {
    let queries = DriverQueriesList.fromJsonString(INSTANCE.generateQueue())
    for (let queryIndex = 0; queryIndex < queries.queries.length; ++queryIndex) {
        // add query name to tags for metrics differentiation
        const metricsTags = {
            "query": queries.queries[queryIndex].name
        }
        const startTime = Date.now()
        const err = INSTANCE.runQuery(DriverQuery.toJsonString(queries.queries[queryIndex]));
        METER_REQUESTS_COUNTER.add(1, metricsTags)
        if (err) {
            METER_REQUEST_ERROR_COUNTER.add(1, metricsTags)
            console.error(queries[queryIndex].name, err)
            throw err
        } else {
            METER_RESPONSES_TIME_TREND.add(Date.now() - startTime, metricsTags)
        }
    }
};

export const teardown = () => {
    const err = INSTANCE.teardown()
    if (err !== undefined) {
        throw err
    }
}

// Summary function, that will create summary file with metrics.
export function handleSummary(runResult: RunResult<Context>) {
    return {
        // use `runResult.withBaggage` to add baggage for result json
        // stdout: runResult.withBaggage({
        //     some: "value",
        // }).toJsonString()
        stdout: runResult.toJsonString()
    };
}

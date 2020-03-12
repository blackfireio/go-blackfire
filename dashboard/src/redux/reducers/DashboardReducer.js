import Immutable from 'immutable';
import DashboardConstants from '../constants/DashboardConstants';

const _state = Immutable.Map({
    loading: false,
    profiling_enabled: false,
    profiling_sample_rate: -1,
    profiles: [],
    profiler_enabling: false,
    profiler_disabling: false,
    profiler_ending: false,
    error: null,
});

function dashboardLoading(state) {
    return state.set('loading', true);
}

function isError(data) {
    return data.status;
}

function disablePropAndSetError(state, prop, data) {
    const is_error = isError(data);

    return state.withMutations((ctx) => {
        ctx
            .set(prop, false)
            .set('error', is_error ? data : null);

        if (!is_error && data.profiling.enabled !== undefined) {
            ctx
                .set('profiling_enabled', data.profiling.enabled)
                .set('profiling_sample_rate', data.profiling.sample_rate)
                .set('profiles', data.profiles._embedded);
        }
    });
}

function enablePropAndClearError(state, prop) {
    return state.withMutations((ctx) => {
        ctx
            .set(prop, true)
            .set('error', null);
    });
}

function dashboardLoaded(state, data) {
    return state.withMutations((ctx) => {
        ctx
            .set('loading', false)
            .set('profiling_enabled', data.profiling.enabled)
            .set('profiling_sample_rate', data.profiling.sample_rate)
            .set('profiles', data.profiles._embedded)
    });
}

function profilerDisabling(state) {
    return enablePropAndClearError(state, 'profiler_disabling');
}

function profilerDisabled(state, data) {
    return disablePropAndSetError(state, 'profiler_disabling', data);
}

function profilerEnabling(state) {
    return enablePropAndClearError(state, 'profiler_enabling');
}

function profilerEnabled(state, data) {
    return disablePropAndSetError(state, 'profiler_enabling', data);
}

function profilerEnding(state) {
    return enablePropAndClearError(state, 'profiler_ending');
}

function profilerEnded(state, data) {
    return disablePropAndSetError(state, 'profiler_ending', data);
}

function clearError(state) {
    return state.set('error', null);
}

export default function DashboardReducer(state = _state, action) {
    switch (action.type) {
        case DashboardConstants.DASHBOARD_LOADING:
            return dashboardLoading(state);
        case DashboardConstants.DASHBOARD_LOADED:
            return dashboardLoaded(state, action.data);
        case DashboardConstants.PROFILER_ENABLING:
            return profilerEnabling(state);
        case DashboardConstants.PROFILER_ENABLED:
            return profilerEnabled(state, action.data);
        case DashboardConstants.PROFILER_DISABLING:
            return profilerDisabling(state);
        case DashboardConstants.PROFILER_DISABLED:
            return profilerDisabled(state, action.data);
        case DashboardConstants.PROFILER_ENDING:
            return profilerEnding(state);
        case DashboardConstants.PROFILER_ENDED:
            return profilerEnded(state, action.data);
        case DashboardConstants.CLEAR_ERROR:
            return clearError(state);
        default:
            return state;
    }
}

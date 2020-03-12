import DashboardConstants from '../constants/DashboardConstants';
import doFetch from '../fetcher';

/*eslint-disable import/prefer-default-export*/

export function clearError() {
    return {
        type: DashboardConstants.CLEAR_ERROR,
    };
}

function dashboardLoaded(response) {
    return {
        type: DashboardConstants.DASHBOARD_LOADED,
        data: response.data,
    };
}

export function loadDashboard(noLoading = false) {
    return (dispatch) => {
        if (!noLoading) {
            dispatch({
                type: DashboardConstants.DASHBOARD_LOADING,
            });
        }

        doFetch('./dashboard_api')
            .then((response) => {
                dispatch(dashboardLoaded(response));
            }, (error) => {
                /*eslint-disable no-console*/
                console.log('Error while fetching API', error);
                /*eslint-enable no-console*/
            })
            .catch((error) => {
                /*eslint-disable no-console*/
                console.log('Error while loading dashboard', error);
                /*eslint-enable no-console*/
            });
    };
}

function craftErrorIfEmpty(response) {
    if (!response) {
        return {
            status: -1,
            title: 'Unable to join server',
            detail: 'No response data',
        };
    }

    const data = response.data;

    if (typeof data === 'string') {
        return {
            status: -1,
            title: 'Incorrect response',
            detail: 'Server returned a non JSON string.',
        };
    }

    return data;
}

export function enableProfiler() {
    return (dispatch) => {
        dispatch({
            type: DashboardConstants.PROFILER_ENABLING,
        });

        doFetch('./enable', 'POST')
            .then((response) => {
                dispatch({
                    type: DashboardConstants.PROFILER_ENABLED,
                    data: response.data,
                });
            }, (error) => {
                dispatch({
                    type: DashboardConstants.PROFILER_ENABLED,
                    data: craftErrorIfEmpty(error.response),
                });
                /*eslint-disable no-console*/
                console.log('Error while enabling profiler', error);
                /*eslint-enable no-console*/
            })
            .catch((error) => {
                /*eslint-disable no-console*/
                console.log('Error after enabling profiler', error);
                /*eslint-enable no-console*/
            });
    };
}

export function disableProfiler() {
    return (dispatch) => {
        dispatch({
            type: DashboardConstants.PROFILER_DISABLING,
        });

        doFetch('./disable', 'POST')
            .then((response) => {
                dispatch({
                    type: DashboardConstants.PROFILER_DISABLED,
                    data: response.data,
                });
            }, (error) => {
                dispatch({
                    type: DashboardConstants.PROFILER_DISABLED,
                    data: craftErrorIfEmpty(error.response),
                });
                /*eslint-disable no-console*/
                console.log('Error while disabling profiler', error);
                /*eslint-enable no-console*/
            })
            .catch((error) => {
                /*eslint-disable no-console*/
                console.log('Error after disabling profiler', error);
                /*eslint-enable no-console*/
            });
    };
}

export function endProfiler() {
    return (dispatch) => {
        dispatch({
            type: DashboardConstants.PROFILER_ENDING,
        });

        doFetch('./end', 'POST')
            .then((response) => {
                dispatch({
                    type: DashboardConstants.PROFILER_ENDED,
                    data: response.data,
                });
            }, (error) => {
                dispatch({
                    type: DashboardConstants.PROFILER_ENDED,
                    data: craftErrorIfEmpty(error.response),
                });
                /*eslint-disable no-console*/
                console.log('Error while ending profiler', error);
                /*eslint-enable no-console*/
            })
            .catch((error) => {
                /*eslint-disable no-console*/
                console.log('Error after ending profiler', error);
                /*eslint-enable no-console*/
            });
    };
}

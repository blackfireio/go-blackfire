import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { connect } from 'react-redux';
import { bindActionCreators } from 'redux';
import * as DashboardActions from '../redux/actions/DashboardActions';
import Error from "./Error";

class ProfilingStatus extends Component {
    handleEnableProfiler = () => {
        this.props.actions.enableProfiler();
    }

    handleDisableProfiler = () => {
        this.props.actions.disableProfiler();
    }

    handleEndProfiler = () => {
        this.props.actions.endProfiler();
    }

    render() {
        const { profiling_enabled, profiling_sample_rate, action_pending } = this.props;

        return (
            <div>
                <h2>{'Configuration:'}</h2>
                <div>
                    <span style={{ verticalAlign: 'middle' }}>{`Profiling is ${profiling_enabled ? 'enabled' : 'disabled'}`}</span>
                    {profiling_enabled && <img alt="" style={{ marginLeft: 10, verticalAlign: 'middle' }} src="data:image/gif;base64,R0lGODlhEAALAPQAAOXl5TIyMsvLy8TExNXV1TU1NTIyMlFRUY2NjXV1dbS0tElJSWVlZZKSknh4eLe3t0xMTDQ0NGhoaNPT08nJydzc3FhYWMzMzNzc3LKysqKiosDAwNnZ2QAAAAAAAAAAACH+GkNyZWF0ZWQgd2l0aCBhamF4bG9hZC5pbmZvACH5BAALAAAAIf8LTkVUU0NBUEUyLjADAQAAACwAAAAAEAALAAAFLSAgjmRpnqSgCuLKAq5AEIM4zDVw03ve27ifDgfkEYe04kDIDC5zrtYKRa2WQgAh+QQACwABACwAAAAAEAALAAAFJGBhGAVgnqhpHIeRvsDawqns0qeN5+y967tYLyicBYE7EYkYAgAh+QQACwACACwAAAAAEAALAAAFNiAgjothLOOIJAkiGgxjpGKiKMkbz7SN6zIawJcDwIK9W/HISxGBzdHTuBNOmcJVCyoUlk7CEAAh+QQACwADACwAAAAAEAALAAAFNSAgjqQIRRFUAo3jNGIkSdHqPI8Tz3V55zuaDacDyIQ+YrBH+hWPzJFzOQQaeavWi7oqnVIhACH5BAALAAQALAAAAAAQAAsAAAUyICCOZGme1rJY5kRRk7hI0mJSVUXJtF3iOl7tltsBZsNfUegjAY3I5sgFY55KqdX1GgIAIfkEAAsABQAsAAAAABAACwAABTcgII5kaZ4kcV2EqLJipmnZhWGXaOOitm2aXQ4g7P2Ct2ER4AMul00kj5g0Al8tADY2y6C+4FIIACH5BAALAAYALAAAAAAQAAsAAAUvICCOZGme5ERRk6iy7qpyHCVStA3gNa/7txxwlwv2isSacYUc+l4tADQGQ1mvpBAAIfkEAAsABwAsAAAAABAACwAABS8gII5kaZ7kRFGTqLLuqnIcJVK0DeA1r/u3HHCXC/aKxJpxhRz6Xi0ANAZDWa+kEAA7AAAAAAAAAAAA" />}
                </div>
                <div>
                    {`Sample rate: ${profiling_sample_rate} Hz`}
                </div>
                <h2>{'Control:'}</h2>
                <div>
                    <button style={{ verticalAlign: 'middle' }} disabled={profiling_enabled} onClick={this.handleEnableProfiler}>{'Enable'}</button>
                    <button style={{ verticalAlign: 'middle' }} disabled={!profiling_enabled} onClick={this.handleDisableProfiler}>{'Disable'}</button>
                    <button style={{ verticalAlign: 'middle' }} disabled={!profiling_enabled} onClick={this.handleEndProfiler}>{'End'}</button>
                    {action_pending && <img alt="" style={{ marginLeft: 10, verticalAlign: 'middle' }} src="data:image/gif;base64,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" />}
                </div>
                <Error />
            </div>
        );
    }
}

ProfilingStatus.propTypes = {
    action_pending: PropTypes.bool.isRequired,
    profiling_enabled: PropTypes.bool.isRequired,
    profiling_sample_rate: PropTypes.number.isRequired,
    actions: PropTypes.shape({
        enableProfiler: PropTypes.func.isRequired,
        disableProfiler: PropTypes.func.isRequired,
        endProfiler: PropTypes.func.isRequired,
    }).isRequired,
};

function mapDispatchToProps(dispatch) {
    return {
        actions: bindActionCreators(DashboardActions, dispatch),
    };
}

export default connect((state) => ({
    action_pending: state.DashboardReducer.get('profiler_enabling') || state.DashboardReducer.get('profiler_disabling') || state.DashboardReducer.get('profiler_ending'),
    profiling_enabled: state.DashboardReducer.get('profiling_enabled'),
    profiling_sample_rate: state.DashboardReducer.get('profiling_sample_rate'),
}), mapDispatchToProps)(ProfilingStatus);

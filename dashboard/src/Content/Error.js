import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { connect } from 'react-redux';
import { bindActionCreators } from 'redux';
import * as DashboardActions from '../redux/actions/DashboardActions';

class Error extends Component {
    timeout;

    componentDidMount() {
        if (this.props.error) {
            this._setTimeout();
        }
    }

    componentDidUpdate(prevProps) {
        if (prevProps.error !== this.props.error) {
            this._clearTimeout();
            if (this.props.error) {
                this._setTimeout();
            }
        }
    }

    componentWillUnmount() {
        this._clearTimeout();
    }

    _setTimeout() {
        this.timeout = setTimeout(() => {
            this.props.actions.clearError();
            this.timeout = null;
        }, 10000);
    }

    _clearTimeout() {
        if (this.timeout) {
            clearTimeout(this.timeout);
        }
        this.timeout = null;
    }

    render() {
        const { error } = this.props;

        if (!error) {
            return null;
        }

        return (
            <div className="error">
                {`${error.title} (${ error.detail})`}
            </div>
        );
    }
}

Error.defaultProps = {
    error: null,
};

Error.propTypes = {
    actions: PropTypes.shape({
        clearError: PropTypes.func.isRequired,
    }).isRequired,
    error: PropTypes.shape({
        status: PropTypes.number.isRequired,
        title: PropTypes.string.isRequired,
        detail: PropTypes.string.isRequired,
    }),
};

function mapDispatchToProps(dispatch) {
    return {
        actions: bindActionCreators(DashboardActions, dispatch),
    };
}

export default connect((state) => ({
    error: state.DashboardReducer.get('error'),
}), mapDispatchToProps)(Error);

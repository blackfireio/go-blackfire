import { bindActionCreators } from 'redux';
import { connect } from 'react-redux';
import PropTypes from 'prop-types';
import React, { Component } from 'react';
import ProfilingStatus from './ProfilingStatus';
import ProfileList from './ProfileList';
import * as DashboardActions from '../redux/actions/DashboardActions';

class Content extends Component {
    constructor(props) {
        super(props);
        this.polling = null;
    }

    componentDidMount() {
        this._doLoad();
        this.poll();
    }

    componentWillUnmount() {
        this.clearInterval();
    }

    _doLoad(noLoading = false) {
        this.props.actions.loadDashboard(noLoading);
    }

    clearInterval() {
        if (this.polling !== null) {
            clearInterval(this.polling);
        }
        this.polling = null;
    }

    poll() {
        this.polling = setInterval(() => {
            this.periodicalPoll();
        }, 1000);
    }

    periodicalPoll() {
        this._doLoad(true);
    }

    render() {
        return (
            <div className="wrapper">
                <ProfilingStatus />
                <ProfileList />
            </div>
        );
    }
}

Content.propTypes = {
    actions: PropTypes.shape({
        loadDashboard: PropTypes.func.isRequired,
    }).isRequired,
};

function mapDispatchToProps(dispatch) {
    return {
        actions: bindActionCreators(DashboardActions, dispatch),
    };
}

export default connect(
    undefined,
    mapDispatchToProps,
)(Content);

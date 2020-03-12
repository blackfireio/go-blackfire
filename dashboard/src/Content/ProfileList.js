import React from 'react';
import PropTypes from 'prop-types';
import { connect } from 'react-redux';
import Timeago from '../Timeago';

function upperCaseFirst(word) {
    if (!word || !word.length || word.length < 1) {
        return word;
    }

    return `${word[0].toUpperCase()}${word.substr(1)}`;
}

function ProfileList({ profiles }) {
    return (
        <div>
            <h2>{'Profiles:'}</h2>
            {profiles.map((profile) => (
                <div key={profile}>
                    <Timeago date={profile.created_at} />
                    {` - ${upperCaseFirst(profile.status)} - `}
                    <a href={profile.url} rel="noopener noreferrer" target="_blank">
                        {profile.name}
                    </a>
                </div>
            ))}
            {profiles.length === 0 ? <i>{'No profiles yet'}</i> : null}
        </div>
    );
}

ProfileList.propTypes = {
    profiles: PropTypes.arrayOf(PropTypes.shape({
        name: PropTypes.string,
    })).isRequired,
};

export default connect((state) => ({
    profiles: state.DashboardReducer.get('profiles'),
}))(ProfileList);

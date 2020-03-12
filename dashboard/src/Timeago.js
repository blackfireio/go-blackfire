import React, { PureComponent } from 'react';
import PropTypes from 'prop-types';
import moment from 'moment';
import TimeagoLib from './Timeagolib';

const monthNames = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
const dayNames = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];

function twoDigits(number) {
    number = `${number}`;

    return number.length === 2 ? number : `0${number}`;
}

export default class Timeago extends PureComponent {
    render() {
        const momentDate = moment(this.props.date);

        if (!momentDate.isValid()) {
            return null;
        }

        const normalizedDate = momentDate.toDate();
        const offset = normalizedDate.getTimezoneOffset() / -60;
        const gmt = offset === 0 ? 'UTC' : (`GMT${(offset > 0 ? '+' : '')}${offset}`);

        return (
            <span
                title={`${dayNames[normalizedDate.getDay()]} ${monthNames[normalizedDate.getMonth()]} ${normalizedDate.getDate()}, ${normalizedDate.getFullYear()}, ${twoDigits(normalizedDate.getHours())}:${twoDigits(normalizedDate.getMinutes())} ${gmt}`}
            >
                {TimeagoLib.timeAgoInWords(this.props.date)}
            </span>
        );
    }
}

Timeago.defaultProps = {
    date: null,
};

Timeago.propTypes = {
    date: PropTypes.string,
};

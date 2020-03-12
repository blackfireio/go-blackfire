// Inspired from
// Copyright 2012, Terry Tai, Pragmatic.ly
// https://pragmatic.ly/
// Licensed under the MIT license.
// https://github.com/pragmaticly/smart-time-ago/blob/master/LICENSE

import moment from 'moment';

function getTimeDistanceInMinutes(absolutTime) {
    const timeDistance = new Date().getTime() - absolutTime.getTime();

    return Math.round((Math.abs(timeDistance) / 1000) / 60);
}

class Timeago {
    constructor() {
        this.options = {
            selector: 'time.timeago',
            attr: 'datetime',
            dir: 'up',
            lang: {
                units: {
                    second: 'second',
                    seconds: 'seconds',
                    minute: 'minute',
                    minutes: 'minutes',
                    hour: 'hour',
                    hours: 'hours',
                    day: 'day',
                    days: 'days',
                    month: 'month',
                    months: 'months',
                    year: 'year',
                    years: 'years',
                },
                prefixes: {
                    lt: 'less than a',
                    about: '', // 'about',
                    over: 'over',
                    almost: 'almost',
                    ago: '',
                },
                suffix: ' ago',
            },
        };
    }

    timeAgoInWords(timeString) {
        const momentDate = moment(timeString);

        if (!momentDate.isValid()) {
            return timeString;
        }

        const absoluteTime = momentDate.toDate();
        const direction = new Date().getTime() - absoluteTime.getTime() > 0;

        return `${this.options.lang.prefixes.ago}${this.distanceOfTimeInWords(absoluteTime)}${direction ? this.options.lang.suffix : ''}`;
    }

    distanceOfTimeInWords(absolutTime) {
        const dim = getTimeDistanceInMinutes(absolutTime);

        if (dim === 0) {
            return `${this.options.lang.prefixes.lt} ${this.options.lang.units.minute}`;
        } if (dim === 1) {
            return `1 ${this.options.lang.units.minute}`;
        } if (dim >= 2 && dim <= 44) {
            return `${dim} ${this.options.lang.units.minutes}`;
        } if (dim >= 45 && dim <= 89) {
            return `${this.options.lang.prefixes.about} 1 ${this.options.lang.units.hour}`;
        } if (dim >= 90 && dim <= 1439) {
            return `${this.options.lang.prefixes.about} ${Math.round(dim / 60)} ${this.options.lang.units.hours}`;
        } if (dim >= 1440 && dim <= 2519) {
            return `1 ${this.options.lang.units.day}`;
        } if (dim >= 2520 && dim <= 43199) {
            return `${Math.round(dim / 1440)} ${this.options.lang.units.days}`;
        } if (dim >= 43200 && dim <= 86399) {
            return `${this.options.lang.prefixes.about} 1 ${this.options.lang.units.month}`;
        } if (dim >= 86400 && dim <= 525599) {
            return `${Math.round(dim / 43200)} ${this.options.lang.units.months}`;
        } if (dim >= 525600 && dim <= 655199) {
            return `${this.options.lang.prefixes.about} 1 ${this.options.lang.units.year}`;
        } if (dim >= 655200 && dim <= 914399) {
            return `${this.options.lang.prefixes.over} 1 ${this.options.lang.units.year}`;
        } if (dim >= 914400 && dim <= 1051199) {
            return `${this.options.lang.prefixes.almost} 2 ${this.options.lang.units.years}`;
        }

        return `${this.options.lang.prefixes.about} ${Math.round(dim / 525600)} ${this.options.lang.units.years}`;
    }
}

export default new Timeago();

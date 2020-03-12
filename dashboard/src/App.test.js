import React from 'react';
import { render } from '@testing-library/react';
import App from './App';

/*eslint-disable no-undef*/

test('renders troubleshooting link', () => {
    const { getByText } = render(<App />);
    const linkElement = getByText(/Troubleshooting/i);
    expect(linkElement).toBeInTheDocument();
});

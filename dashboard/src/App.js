import React from 'react';
import './App.css';
import { Provider } from 'react-redux';
import store from './redux/stores/configureStore';
import BlackfireLogo from './Icon/BlackfireLogo';
import Content from './Content';

function App() {
    return (
        <Provider store={store}>
            <header className="App-header">
                <div className="wrapper">
                    <BlackfireLogo style={{ width: 300 }} />
                </div>
            </header>
            <Content />
            <footer className="App-footer">
                <div className="wrapper">
                    <p>
                        {'Blackfire Go Probe Dashboard version experimental - '}
                        <a href="https://blackfire.io/docs/troubleshooting">{'Troubleshooting'}</a>
                    </p>
                </div>
            </footer>
        </Provider>
    );
}

export default App;

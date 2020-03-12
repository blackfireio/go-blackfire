/*eslint-disable global-require*/

import { createStore, applyMiddleware, compose } from 'redux';
import thunk from 'redux-thunk';
import createRootReducer from '../reducers';

export default function configureStore(initialState) {
    const store = createStore(
        createRootReducer(), // root reducer with router state
        initialState,
        compose(
            applyMiddleware(
                thunk,
            ),
        ),
    );

    return store;
}

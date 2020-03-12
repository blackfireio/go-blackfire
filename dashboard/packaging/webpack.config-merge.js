const UglifyJsPlugin = require('uglifyjs-webpack-plugin');
const path = require('path');
const glob = require('glob');

module.exports = {
    mode: 'production',
    entry: {
        'bundle.js': glob.sync(`${__dirname}/../build/static/?(js|css)/*.?(js|css)`).map((f) => path.resolve(__dirname, f)),
    },
    output: {
        path: `${__dirname}/../build/merged`,
        filename: 'bundle.js',
    },
    module: {
        rules: [
            {
                test: /\.css$/,
                use: ['style-loader', 'css-loader'],
            },
        ],
    },
    optimization: {
        minimizer: [new UglifyJsPlugin()],
    },
};


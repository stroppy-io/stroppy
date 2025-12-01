const path = require('path');

module.exports = {
    entry: "./analyze_ddl_entry.ts",
    mode: 'production',
    module: {
        rules: [
            {
                test: /\.tsx?$/,
                use: {
                    loader: 'ts-loader',
                    options: { transpileOnly: true }
                },
                exclude: /node_modules/,
            },
        ],
    },
    resolve: {
        extensions: ['.tsx', '.ts', '.js'],
    },
    externalsType: 'commonjs',
    externals: [/^k6(\/.*)?$/, './stroppy.pb.js'],
    output: {
        filename: 'analyze_ddl.js',
        path: path.resolve(__dirname, 'dist'),
        libraryTarget: 'module',
    },
    experiments: {
        outputModule: true,
    },
};

export type ResultType = {
    name: string,
    pass: boolean,
    result: any
};

export type TestRun = () => Promise<ResultType>;
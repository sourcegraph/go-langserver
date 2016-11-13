export class Logger {
    private filename: string;
    public constructor(filename: string) {
        this.filename = filename;
    }

    private getMsgPrefix(msg: string) {
        let prefix = `${this.filename}\n${msg}`;
        return prefix;
    }

    public log(msg: string, ...items: any[]) {
        console.info(this.getMsgPrefix(msg), ...items, '\n');
    }
}

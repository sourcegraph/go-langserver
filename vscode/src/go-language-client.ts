import Uri from 'vscode-uri';
import {
    LanguageClient,
    LanguageClientOptions,
    ServerOptions, TransportKind, ExecutableOptions,
    SettingMonitor,
    ErrorAction, ErrorHandler,
    CloseAction,
} from 'vscode-languageclient';

// TODO: extend LanguageClient 

/**
 * GoLanguageClient
 */
export class GoLanguageClient {
    private languageClient: LanguageClient;
    public constructor() {
        this.init();
    }

    public start(): any {
        return this.languageClient.start();
    }

    private init() {
        let serverOptions: ServerOptions = this.createServerOptions();
        let clientOptions: LanguageClientOptions = this.createLanguageClientOptions();
        this.languageClient = new LanguageClient(
            'langserver-antha',
            serverOptions,
            clientOptions
        );
    }

    private createServerOptions(): ServerOptions {
        // const TRANSPORT_KEY = TransportKind.constructor.name;
        const TRANSPORT_KEY = 'TransportKind';

        let options: ExecutableOptions = {
            // cwd: '',
            // stdio: '', // ['', ...]
            env: {
                [TRANSPORT_KEY]: TransportKind.websocket
            },
            detached: false
        };
        return {
            command: 'langserver-antha',
            args: [
                '-mode=ws',
                // Uncomment for verbose logging to the vscode
                // "Output" pane and to a temporary file:
                '-trace', '-logfile=/tmp/langserver-go.log'
            ],
            options
        };
    }

    private createLanguageClientOptions(): LanguageClientOptions {
        return {
            documentSelector: ['go'],
            uriConverters: {
                // Apply file:/// scheme to all file paths.
                code2Protocol: (uri: Uri): string => (uri.scheme ? uri : uri.with({ scheme: 'file' })).toString(),
                protocol2Code: (uri: string) => Uri.parse(uri)
            }
        };
    }
}
import Uri from 'vscode-uri';
import {
    LanguageClient,
    LanguageClientOptions, Configuration, SynchronizeOptions, RevealOutputChannelOn,
    InitializationFailedHandler, InitializeError,
    ErrorHandler,
    ServerOptions, TransportKind, ExecutableOptions,
    SettingMonitor,
    ErrorAction,
    CloseAction
} from 'vscode-languageclient';
import {
    Message,
    ResponseError
} from 'vscode-jsonrpc';
import * as WebSocket from 'ws'

// TODO: extend LanguageClient 

/**
 * GoLanguageClient
 */
export class GoLanguageClient {
    private languageClient: LanguageClient;
    private initializationFailedHandler: InitializationFailedHandler;
    private errorHandler: ErrorHandler;

    public constructor() {
        this.init();
    }

    public start(): any {
        return this.languageClient.start();
    }

    private init() {
        this.createHandlers();

        let serverOptions: ServerOptions = this.createServerOptions();
        let clientOptions: LanguageClientOptions = this.createLanguageClientOptions();
        this.languageClient = new LanguageClient(
            'langserver-antha',
            serverOptions,
            clientOptions
        );
    }

    private createServerOptions(): ServerOptions {
    
        let transport: TransportKind = TransportKind.websocket;
        let options: ExecutableOptions = {
            // cwd: '',
            // stdio: '', // ['', ...]
            detached: true
        };
        return {
            command: 'langserver-antha',
            args: [
                '-mode', 'ws',
                // Uncomment for verbose logging to the vscode
                // "Output" pane and to a temporary file:
                '-trace',
                '-logfile', '/tmp/langserver-antha.log'
            ],
            transport,
            options
        };
    }

    private createLanguageClientOptions(): LanguageClientOptions {
        let initializationFailedHandler = this.initializationFailedHandler;
        let errorHandler = this.errorHandler;
        let revealOutputChannelOn: RevealOutputChannelOn = RevealOutputChannelOn.Info;

        return {
            documentSelector: ['go'],
            revealOutputChannelOn,
            initializationFailedHandler,
            errorHandler,
            uriConverters: {
                // Apply file:/// scheme to all file paths.
                code2Protocol: (uri: Uri): string => (uri.scheme ? uri : uri.with({ scheme: 'file' })).toString(),
                protocol2Code: (uri: string) => Uri.parse(uri)
            }
        };
    }

    private createHandlers(): void {
        this.initializationFailedHandler = this.createInitializationFailedHandler();
        this.errorHandler = this.createErrorHandler();
    }

    private createInitializationFailedHandler(): InitializationFailedHandler {
        return (error: ResponseError<InitializeError> | Error | any): boolean => {
            // return false to terminate the LanguageClient
            return false;
        };
    }

    private createErrorHandler(): ErrorHandler {
        // shutdown then restart the server...
        return {
            error(error: Error, message: Message, count: number): ErrorAction {
                return ErrorAction.Shutdown;
            },
            closed(): CloseAction {
                return CloseAction.Restart;
            }
        };
    }
}
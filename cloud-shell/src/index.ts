/*
 * Copyright (c) 2019 Red Hat, Inc.
 * All rights reserved. This program and the accompanying materials are made
 * available under the terms of the Eclipse Public License 2.0
 * which is available at https://www.eclipse.org/legal/epl-2.0/
 *
 * Contributors:
 *   Red Hat, Inc. - initial API and implementation
 */

import { CloudShellTerminal, TerminalHandler } from "./terminal";
import { JsonRpcConnection } from "./json-rpc-connection";
import { GenericNotificationHandler } from "vscode-jsonrpc";
import { MachineExec } from "./terminal-protocol";

const terminalElem = document.getElementById('terminal-container');

const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws';
const port = window.location.port ? `:${window.location.port}` : '';
const hostUrl = `${protocol}://${window.location.host}${port}`;
const connectUrl = hostUrl + '/connect';
const attachUrl = hostUrl + '/attach';

const terminal: CloudShellTerminal = new CloudShellTerminal();

terminal.open(terminalElem);
terminal.sendLine('Welcome to the Cloud Shell.');

console.log(connectUrl);
const rpcConnecton = new JsonRpcConnection(connectUrl);
rpcConnecton.create().then(connection => {
    connection.onNotification('connected', (handler: GenericNotificationHandler) => {
        const exec: MachineExec = {
            tty: true,
            cols: terminal.cols,
            rows: terminal.rows,
        };

        connection.sendRequest<{}>('create', exec).then((value: {}) => {
            const id = value as number;
            const attachConnection = rpcConnecton.createReconnectionWebsocket(`${attachUrl}/${id}`);

            attachConnection.onopen = (event: Event) => {
                // resize terminal first time on open
                connection.sendRequest('resize', {cols: terminal.cols, rows: terminal.rows, id});

                attachConnection.onmessage = (event: MessageEvent) => {
                    terminal.sendText(event.data);
                }

                const terminalHandler: TerminalHandler = {
                    onData(data: string):void {
                        attachConnection.send(data);
                    },
                    onResize(cols: number, rows: number) {
                        connection.sendRequest('resize', {cols, rows, id});
                    }
                }

                terminal.addHandler(terminalHandler);
            };
            attachConnection.onerror = (errEvn: ErrorEvent) => {
                console.log('Attach connection error: ', errEvn.error);
            }
            attachConnection.onclose = (event: CloseEvent) => {
                console.log('Attach connection closed: ', event.code);
            }
        });
    });
}).catch(err => {
    console.log('Fatal. Unable to connect to container.', err);
})

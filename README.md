# Nuntius for Go
Nuntius for Go is the client for a seamless connection service that allows to connect devices even behind NAT.

# How it works
A client connects to the central server to await a connection from another client. When the second client is connected, it requests to be connected with the first client. After this request, new connections are spawned by both clients and a "tunnel" created in the server to link both connections.

# Change Log

- 2018/01 - Initial version

# Trivia
Nuntius is the Latin word for Messenger.

# License
The Nuntius for Go project is released under the MIT license. For the full license see LICENSE file.

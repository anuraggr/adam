# adam

A high-performance CLI download manager written in Go.

`adam` is designed to download files with **multiple parallel workers** using **HTTP range requests**. It bypasses the single-threaded bottleneck of standard browsers/tools by splitting files into chunks and downloading them concurrently, maximizing available bandwidth.

## Features

- **Parallel Chunk Downloading:** Splits files into multiple parts using HTTP Range headers to saturate the connection.
- **Resumable Downloads:** State tracking allows you to pause and resume downloads without restarting from zero.
- **Concurrency Control:** Efficiently manages Go routines to handle multiple worker threads.
- **Cross-Platform:** Single binary executable for Linux, Windows, and macOS.

## Installation

### Option 1: Using Go (Recommended)
If you have Go installed, you can install `adam` directly:

~~~bash
go install github.com/anuraggr/adam@latest
~~~
*Make sure your `$(go env GOPATH)/bin` is in your system `PATH`.*

### Option 2: Build from Source
~~~bash
git clone https://github.com/anuraggr/adam.git
cd adam
go build -o adam
~~~

## Usage

**Start Download:**
~~~bash
adam <url>
~~~

**View the status of all current and past downloads:**
~~~bash
adam ls
~~~

**Resume a download:**
~~~bash
adam resume <ID>
~~~

## Architecture

~~~mermaid
graph TD
    Server[Remote Server]
    subgraph adam_Client [adam Client]
        W1[Worker 1: Bytes 0-25%]
        W2[Worker 2: Bytes 26-50%]
        W3[Worker 3: Bytes 51-75%]
        W4[Worker 4: Bytes 76-100%]
        Merger[File Assembler]
    end
    
    Server -- Range Request --> W1
    Server -- Range Request --> W2
    Server -- Range Request --> W3
    Server -- Range Request --> W4
    W1 --> Merger
    W2 --> Merger
    W3 --> Merger
    W4 --> Merger
    Merger --> Disk[(Local Disk)]
~~~

const path = require("path");
const execSync = require("child_process").execSync;

export function runCommand(command: string, options?: any, log = true): any{
	let output = {
        stdout: null,
        error: null,
    };
    try {
        const shell_output = execSync(command, options);
        if (shell_output) {
            output.stdout = shell_output.toString().trim();
        }
    } catch (error) {
        output.error = error;
    }
    if (log) {
        if (output.stdout) {
            console.log(`Running shell command ${command}, output is ${output.stdout}.`);
        } else if (output.error) {
            console.error(`Running shell command ${command}, error is ${output.stdout}.`);
        } else {
            console.log(`Running shell command ${command}, output is blank.`);
        }
    }
    return output;
}

export function getGitCommitHash(directory: string) {
    const command = `cd ${directory} && git rev-parse HEAD`;
    const output = runCommand(command);
    if (output.stdout && !output.error) {
        return output.stdout;
    }
    return null;
}

function hasFileChanged(directory, filename) {
    const command = `cd ${directory} && git diff ${filename}`;
    const output = runCommand(command);
    if (output.stdout !== "") {
        return true;
    } else if (output.error) {
        return null;
    }
    return false;
}

export function getTopLevelGitDirectory(directory: string) : string|null {
    const command = `cd ${directory} && git rev-parse --show-toplevel`;
    const output = runCommand(command);
    if (output.stdout && !output.error) {
        return output.stdout;
    }
    return null;
}

export function getGitUrl(directory: string) : string|null {
    const command = `cd ${directory} && git config --get remote.origin.url`;
    const output = runCommand(command);
    if (output.stdout && !output.error) {
        return output.stdout;
    }
    return null;
}

export function cleanGitUrl(url: string) :string {
    const clean = new RegExp("git\@|.git|https:\/\/", "g");
    return url.replace(clean, "").split("com:").join("com/");
}

function getGitStatus(data) {
    const parentDirectory = path.resolve(data.filename, "..").split(" ").join("\\ ");
    const basename = path.basename(data.filename);

    // if file has changed, return because annotations won't correlate
    if (data.is_dirty) {
        return null;
    }
    const fileChanged = this.hasFileChanged(parentDirectory, basename);
    if (fileChanged) {
        return null;
    }

    let gitCommitID = this.getGitCommitHash(parentDirectory);

    const mainGitRepo = this.getTopLevelGitDirectory(parentDirectory);

    let gitWebURI = this.getGitUrl(parentDirectory);
    gitWebURI = this.cleanGitUrl(gitWebURI);

    const relativeGitRepo = parentDirectory.split(mainGitRepo).join("");
    const relativeFilePath = data.filename.split(`${mainGitRepo}/`).join("");

    return {gitWebUri: gitWebURI, relativeGitRepo: relativeGitRepo, relativeFilePath: relativeFilePath, gitCommitId: gitCommitID};

}

export function getGitRepoNameFromUrl(gitUrl: string): string{
    const repo_parts = gitUrl.split(path.sep);
    return repo_parts[repo_parts.length-1];
}

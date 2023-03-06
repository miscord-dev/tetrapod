const getVersion = () => {
    const byohVersion = 
        (`${process.env.GITHUB_REF}`.match(/(v\d+\.\d+\.\d+)/) ?? [])[0];

    return byohVersion;
}

exports.release = async (component) => {
    await $`make release IMG=ghcr.io/miscord-dev/tetrapod-${component}:${getVersion()}`
}

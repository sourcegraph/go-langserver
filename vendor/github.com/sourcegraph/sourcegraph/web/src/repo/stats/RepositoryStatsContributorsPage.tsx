import { escapeRegExp, isEqual } from 'lodash'
import * as React from 'react'
import { Link, RouteComponentProps } from 'react-router-dom'
import { Observable, Subject } from 'rxjs'
import { map } from 'rxjs/operators'
import { gql } from '../../../../shared/src/graphql/graphql'
import * as GQL from '../../../../shared/src/graphql/schema'
import { createAggregateError } from '../../../../shared/src/util/errors'
import { queryGraphQL } from '../../backend/graphql'
import { FilteredConnection } from '../../components/FilteredConnection'
import { Form } from '../../components/Form'
import { PageTitle } from '../../components/PageTitle'
import { Timestamp } from '../../components/time/Timestamp'
import { buildSearchURLQuery, quoteIfNeeded, searchQueryForRepoRev } from '../../search'
import { eventLogger } from '../../tracking/eventLogger'
import { PersonLink } from '../../user/PersonLink'
import { UserAvatar } from '../../user/UserAvatar'
import { memoizeObservable } from '../../util/memoize'
import { numberWithCommas, pluralize } from '../../util/strings'
import { RepositoryStatsAreaPageProps } from './RepositoryStatsArea'

interface QuerySpec {
    revisionRange: string | null
    after: string | null
    path: string | null
}

interface RepositoryContributorNodeProps extends QuerySpec {
    node: GQL.IRepositoryContributor
    repoPath: string
}

const RepositoryContributorNode: React.FunctionComponent<RepositoryContributorNodeProps> = ({
    node,
    repoPath,
    revisionRange,
    after,
    path,
}) => {
    const commit = node.commits.nodes[0] as GQL.IGitCommit | undefined

    const query: string = [
        searchQueryForRepoRev(repoPath),
        'type:diff',
        `author:${quoteIfNeeded(node.person.email)}`,
        after ? `after:${quoteIfNeeded(after)}` : '',
        path ? `file:${quoteIfNeeded(escapeRegExp(path))}` : '',
    ]
        .join(' ')
        .replace(/\s+/, ' ')

    return (
        <div className="repository-contributor-node list-group-item py-2">
            <div className="repository-contributor-node__person">
                <UserAvatar className="icon-inline mr-2" user={node.person} />
                <PersonLink userClassName="font-weight-bold" {...node.person} />
            </div>
            <div className="repository-contributor-node__commits">
                <div className="repository-contributor-node__commit">
                    {commit && (
                        <>
                            <Timestamp date={commit.author.date} />:{' '}
                            <Link
                                to={commit.url}
                                className="repository-contributor-node__commit-subject"
                                data-tooltip="Most recent commit by contributor"
                            >
                                {commit.subject}
                            </Link>
                        </>
                    )}
                </div>
                <div className="repository-contributor-node__count">
                    <Link
                        to={`/search?${buildSearchURLQuery({
                            query,
                        })}`}
                        className="font-weight-bold"
                        data-tooltip={
                            revisionRange &&
                            revisionRange.includes('..') &&
                            'All commits will be shown (revision end ranges are not yet supported)'
                        }
                    >
                        {numberWithCommas(node.count)} {pluralize('commit', node.count)}
                    </Link>
                </div>
            </div>
        </div>
    )
}

const queryRepositoryContributors = memoizeObservable(
    (args: {
        repo: GQL.ID
        first?: number
        revisionRange?: string
        after?: string
        path?: string
    }): Observable<GQL.IRepositoryContributorConnection> =>
        queryGraphQL(
            gql`
                query RepositoryContributors(
                    $repo: ID!
                    $first: Int
                    $revisionRange: String
                    $after: String
                    $path: String
                ) {
                    node(id: $repo) {
                        ... on Repository {
                            contributors(first: $first, revisionRange: $revisionRange, after: $after, path: $path) {
                                nodes {
                                    person {
                                        name
                                        displayName
                                        email
                                        avatarURL
                                        user {
                                            username
                                            url
                                        }
                                    }
                                    count
                                    commits(first: 1) {
                                        nodes {
                                            oid
                                            abbreviatedOID
                                            url
                                            subject
                                            author {
                                                date
                                            }
                                        }
                                    }
                                }
                                totalCount
                                pageInfo {
                                    hasNextPage
                                }
                            }
                        }
                    }
                }
            `,
            args
        ).pipe(
            map(({ data, errors }) => {
                if (!data || !data.node || !(data.node as GQL.IRepository).contributors || errors) {
                    throw createAggregateError(errors)
                }
                return (data.node as GQL.IRepository).contributors
            })
        ),
    args => `${args.repo}:${args.first}:${args.revisionRange}:${args.after}:${args.path}`
)

interface Props extends RepositoryStatsAreaPageProps, RouteComponentProps<{}> {}

class FilteredContributorsConnection extends FilteredConnection<
    GQL.IRepositoryContributor,
    Pick<RepositoryContributorNodeProps, 'repoPath' | 'revisionRange' | 'after' | 'path'>
> {}

interface State extends QuerySpec {}

/** A page that shows a repository's contributors. */
export class RepositoryStatsContributorsPage extends React.PureComponent<Props, State> {
    private static REVISION_RANGE_INPUT_ID = 'repository-stats-contributors-page__revision-range'
    private static AFTER_INPUT_ID = 'repository-stats-contributors-page__after'
    private static PATH_INPUT_ID = 'repository-stats-contributors-page__path'

    public state: State = this.getDerivedProps()

    private specChanges = new Subject<void>()

    public componentDidMount(): void {
        eventLogger.logViewEvent('RepositoryStatsContributors')
    }

    public componentDidUpdate(prevProps: Props): void {
        const spec = this.getDerivedProps(this.props.location)
        const prevSpec = this.getDerivedProps(prevProps.location)
        if (!isEqual(spec, prevSpec)) {
            eventLogger.log('RepositoryStatsContributorsPropsUpdated')
            this.setState(spec)
            this.specChanges.next()
        }
    }

    private getDerivedProps(location = this.props.location): QuerySpec {
        const query = new URLSearchParams(location.search)
        const revisionRange = query.get('revision-range')
        const after = query.get('after')
        const path = query.get('path')
        return { revisionRange, after, path }
    }

    public render(): JSX.Element | null {
        const { revisionRange, after, path } = this.getDerivedProps()

        // Whether the user has entered new option values that differ from what's in the URL query and has not yet
        // submitted the form.
        const equalOrEmpty = (a: string | null, b: string | null): boolean => a === b || (!a && !b)
        const stateDiffers =
            !equalOrEmpty(this.state.revisionRange, revisionRange) ||
            !equalOrEmpty(this.state.after, after) ||
            !equalOrEmpty(this.state.path, path)

        return (
            <div className="repository-stats-page">
                <PageTitle title="Contributors" />
                <div className="card repository-stats-page__card repository-stats-page__card--form">
                    <div className="card-header">Contributions filter</div>
                    <div className="card-body">
                        <Form onSubmit={this.onSubmit}>
                            <div className="repository-stats-page__row form-inline">
                                <div className="input-group mb-2 mr-sm-2">
                                    <div className="input-group-prepend">
                                        <label
                                            htmlFor={RepositoryStatsContributorsPage.AFTER_INPUT_ID}
                                            className="input-group-text"
                                        >
                                            Time period
                                        </label>
                                    </div>
                                    <input
                                        type="text"
                                        className="form-control"
                                        name="after"
                                        size={12}
                                        id={RepositoryStatsContributorsPage.AFTER_INPUT_ID}
                                        value={this.state.after || ''}
                                        placeholder="All time"
                                        onChange={this.onChange}
                                    />
                                    <div className="input-group-append">
                                        <div className="btn-group">
                                            <button
                                                type="button"
                                                className={`btn btn-secondary ${
                                                    this.state.after === '7 days ago' ? 'active' : ''
                                                } repository-stats-page__btn-no-left-rounded-corners`}
                                                // tslint:disable-next-line:jsx-no-lambda
                                                onClick={() => this.setStateAfterAndSubmit('7 days ago')}
                                            >
                                                Last 7 days
                                            </button>
                                            <button
                                                type="button"
                                                className={`btn btn-secondary ${
                                                    this.state.after === '30 days ago' ? 'active' : ''
                                                }`}
                                                // tslint:disable-next-line:jsx-no-lambda
                                                onClick={() => this.setStateAfterAndSubmit('30 days ago')}
                                            >
                                                Last 30 days
                                            </button>
                                            <button
                                                type="button"
                                                className={`btn btn-secondary ${
                                                    this.state.after === '1 year ago' ? 'active' : ''
                                                }`}
                                                // tslint:disable-next-line:jsx-no-lambda
                                                onClick={() => this.setStateAfterAndSubmit('1 year ago')}
                                            >
                                                Last year
                                            </button>
                                            <button
                                                type="button"
                                                className={`btn btn-secondary ${!this.state.after ? 'active' : ''}`}
                                                // tslint:disable-next-line:jsx-no-lambda
                                                onClick={() => this.setStateAfterAndSubmit(null)}
                                            >
                                                All time
                                            </button>
                                        </div>
                                    </div>
                                </div>
                            </div>
                            <div className="repository-stats-page__row form-inline">
                                <div className="input-group mt-2 mr-sm-2">
                                    <div className="input-group-prepend">
                                        <label
                                            htmlFor={RepositoryStatsContributorsPage.REVISION_RANGE_INPUT_ID}
                                            className="input-group-text"
                                        >
                                            Revision range
                                        </label>
                                    </div>
                                    <input
                                        type="text"
                                        className="form-control"
                                        name="revision-range"
                                        size={18}
                                        id={RepositoryStatsContributorsPage.REVISION_RANGE_INPUT_ID}
                                        value={this.state.revisionRange || ''}
                                        placeholder="Default branch"
                                        onChange={this.onChange}
                                        autoCapitalize="off"
                                        autoCorrect="off"
                                        autoComplete="off"
                                        spellCheck={false}
                                    />
                                </div>
                                <div className="input-group mt-2 mr-sm-2">
                                    <div className="input-group-prepend">
                                        <label
                                            htmlFor={RepositoryStatsContributorsPage.PATH_INPUT_ID}
                                            className="input-group-text"
                                        >
                                            Path
                                        </label>
                                    </div>
                                    <input
                                        type="text"
                                        className="form-control"
                                        name="path"
                                        size={18}
                                        id={RepositoryStatsContributorsPage.PATH_INPUT_ID}
                                        value={this.state.path || ''}
                                        placeholder="All files"
                                        onChange={this.onChange}
                                        autoCapitalize="off"
                                        autoCorrect="off"
                                        autoComplete="off"
                                        spellCheck={false}
                                    />
                                </div>
                                {stateDiffers && (
                                    <div className="form-group mb-0">
                                        <button type="submit" className="btn btn-primary mr-2 mt-2">
                                            Update
                                        </button>
                                        <button type="reset" className="btn btn-secondary mt-2" onClick={this.onCancel}>
                                            Cancel
                                        </button>
                                    </div>
                                )}
                            </div>
                        </Form>
                    </div>
                </div>
                <FilteredContributorsConnection
                    listClassName="list-group list-group-flush"
                    noun="contributor"
                    pluralNoun="contributors"
                    queryConnection={this.queryRepositoryContributors}
                    nodeComponent={RepositoryContributorNode}
                    nodeComponentProps={{
                        repoPath: this.props.repo.name,
                        revisionRange,
                        after,
                        path,
                    }}
                    defaultFirst={20}
                    hideSearch={true}
                    shouldUpdateURLQuery={false}
                    updates={this.specChanges}
                    history={this.props.history}
                    location={this.props.location}
                />
            </div>
        )
    }

    private onChange: React.ChangeEventHandler<HTMLInputElement> = e => {
        switch (e.currentTarget.id) {
            case RepositoryStatsContributorsPage.REVISION_RANGE_INPUT_ID:
                this.setState({ revisionRange: e.currentTarget.value })
                return

            case RepositoryStatsContributorsPage.AFTER_INPUT_ID:
                this.setState({ after: e.currentTarget.value })
                return

            case RepositoryStatsContributorsPage.PATH_INPUT_ID:
                this.setState({ path: e.currentTarget.value })
                return
        }
    }

    private onSubmit: React.FormEventHandler<HTMLFormElement> = e => {
        e.preventDefault()
        this.props.history.push({ search: this.urlQuery(this.state) })
    }

    private onCancel: React.MouseEventHandler<HTMLButtonElement> = e => {
        e.preventDefault()
        this.setState(this.getDerivedProps())
    }

    private queryRepositoryContributors = (args: { first?: number }) => {
        const { revisionRange, after, path } = this.getDerivedProps()
        return queryRepositoryContributors({
            ...args,
            repo: this.props.repo.id,
            revisionRange: revisionRange || undefined,
            after: after || undefined,
            path: path || undefined,
        })
    }

    private urlQuery(spec: QuerySpec): string {
        const query = new URLSearchParams()
        if (spec.revisionRange) {
            query.set('revision-range', spec.revisionRange)
        }
        if (spec.after) {
            query.set('after', spec.after)
        }
        if (spec.path) {
            query.set('path', spec.path)
        }
        return query.toString()
    }

    private setStateAfterAndSubmit(after: string | null): void {
        this.setState({ after }, () => this.props.history.push({ search: this.urlQuery(this.state) }))
    }
}

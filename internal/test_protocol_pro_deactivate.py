"""Offline deletion contract tests; never contact an upstream service."""
import unittest
from protocol_pro_deactivate import run, BASE, ACCOUNTS

class Response:
    def __init__(self, status=200, data=None):
        self.status_code, self.data = status, data or {}
    def json(self):
        return self.data

class Cookies:
    def set(self, *args, **kwargs):
        pass

class Session:
    def __init__(self):
        self.headers, self.cookies, self.calls = {}, Cookies(), []
        self.email = 'pro@example.com'
        self.me_status = self.account_status = self.post_status = 200
        self.post_data = {'status': 'queued'}
        self.deactivated = False
        self.transport_error = False
        self.personal = True
    def get(self, url, **kwargs):
        self.calls.append(('GET', url, kwargs))
        if url == BASE + '/backend-api/me':
            return Response(self.me_status, {'email': self.email, 'id': 'user-1'})
        assert url == BASE + ACCOUNTS
        account = {'account_id': 'personal-1', 'structure': 'personal' if self.personal else 'workspace', 'account_user_role': 'account-owner', 'is_deactivated': self.deactivated}
        return Response(self.account_status, {'accounts': {'default': {'account': account}, 'personal-1': {'account': account}}})
    def post(self, url, **kwargs):
        self.calls.append(('POST', url, kwargs))
        if self.transport_error:
            raise ConnectionError('private proxy credentials')
        return Response(self.post_status, self.post_data)

class DeactivateTests(unittest.TestCase):
    def perform(self, session, **payload):
        self.events = []
        def call(name, args):
            self.events.append(name)
            if name == 'submitting':
                self.assertFalse(any(x[0] == 'POST' for x in session.calls))
        return run({'email': 'pro@example.com', 'access_token': 'fixture-at', 'user_id': 'user-1', **payload}, call, session, lambda *a: 'fixture-sentinel')

    def test_queued_empty_body_identity_and_write_ahead(self):
        session = Session()
        result = self.perform(session)
        self.assertEqual(result['status'], 'queued')
        self.assertEqual(self.events, ['identity', 'stage', 'submitting', 'accepted'])
        method, url, kwargs = session.calls[-1]
        self.assertEqual((method, url, kwargs['data']), ('POST', BASE + '/backend-api/accounts/deactivate', b''))
        self.assertEqual(kwargs['headers']['chatgpt-account-id'], 'personal-1')
        self.assertFalse(kwargs['allow_redirects'])

    def test_wrong_identity_or_nonpersonal_never_submits(self):
        for mode in ('email', 'user', 'workspace'):
            with self.subTest(mode=mode):
                session = Session()
                extra = {}
                if mode == 'email': session.email = 'other@example.com'
                if mode == 'user': extra['user_id'] = 'other-user'
                if mode == 'workspace': session.personal = False
                self.assertEqual(self.perform(session, **extra)['status'], 'failed')
                self.assertFalse(any(x[0] == 'POST' for x in session.calls))

    def test_401_requests_login_403_does_not_repeat(self):
        for status, expected in ((401, 'need_login'), (403, 'failed')):
            session = Session(); session.me_status = status
            self.assertEqual(self.perform(session)['status'], expected)
            self.assertEqual(len(session.calls), 1)

    def test_unknown_transport_and_server_errors_are_never_retried(self):
        for status in (0, 500, 522):
            session = Session(); session.post_status = status; session.transport_error = status == 0
            result = self.perform(session)
            self.assertEqual(result['status'], 'unknown')
            self.assertNotIn('private', result['message'])
            self.assertEqual(sum(x[0] == 'POST' for x in session.calls), 1)
            self.assertNotIn('rejected', self.events)

    def test_check_only_never_posts_and_401_is_not_confirmation(self):
        for status, deactivated, expected in ((200, True, 'confirmed'), (200, False, 'pending'), (401, False, 'pending'), (403, False, 'pending')):
            session = Session(); session.account_status = status; session.deactivated = deactivated
            self.assertEqual(self.perform(session, check_only=True, account_id='personal-1')['status'], expected)
            self.assertEqual(len(session.calls), 1)
            self.assertEqual(session.calls[0][1], BASE + ACCOUNTS)
            self.assertEqual(self.events, [])

    def test_checkpoint_failure_prevents_post(self):
        session = Session()
        def call(name, args):
            if name == 'submitting': raise RuntimeError('checkpoint failed')
        result = run({'email': 'pro@example.com', 'access_token': 'fixture-at'}, call, session, lambda *a: 'fixture')
        self.assertEqual(result['status'], 'failed')
        self.assertFalse(any(x[0] == 'POST' for x in session.calls))

    def test_client_rejection_and_unrecognized_success(self):
        for code, data, expected in ((403, {}, 'failed'), (200, {}, 'unknown')):
            session = Session(); session.post_status, session.post_data = code, data
            self.assertEqual(self.perform(session)['status'], expected)
            self.assertEqual('rejected' in self.events, code == 403)

if __name__ == '__main__':
    unittest.main()
